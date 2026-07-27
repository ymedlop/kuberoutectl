package aws

import (
	"reflect"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// credentialRank must be a total order over every health the domain defines,
// best first. A value it does not know must sort last rather than panic or
// collide with a real rank — a new AccessHealth added later should degrade to
// "worst", never silently outrank a working credential.
func TestCredentialRankIsTotalAndKnowsEveryHealth(t *testing.T) {
	ordered := []domain.AccessHealth{
		domain.HealthValid,
		domain.HealthExpiring,
		domain.HealthStatic,
		domain.HealthExpired,
		domain.HealthError,
		domain.HealthUnknown,
	}
	for i := 1; i < len(ordered); i++ {
		if credentialRank(ordered[i-1]) >= credentialRank(ordered[i]) {
			t.Errorf("rank(%s)=%d must be better (lower) than rank(%s)=%d",
				ordered[i-1], credentialRank(ordered[i-1]), ordered[i], credentialRank(ordered[i]))
		}
	}
	if got, want := credentialRank("something-new"), credentialRank(domain.HealthUnknown); got <= want {
		t.Errorf("rank of an unrecognised health = %d, want worse than unknown (%d)", got, want)
	}
}

// foldCandidate builds a target the way buildTarget does, so a fold test
// exercises structs with the same internal consistency real discovery produces.
func foldCandidate(profile, arn string, health domain.AccessHealth, action domain.ActionHint) domain.Target {
	return buildTarget(
		profile, "eu-central-1",
		awsIdentity{Account: "111122223333", Arn: "arn:aws:iam::111122223333:user/" + profile},
		awsCluster{Name: "prod-eks", Arn: arn, Endpoint: "https://prod.eks.example", Version: "1.29", Status: "ACTIVE"},
		health, action, time.Unix(0, 0).UTC(),
	)
}

const sharedARN = "arn:aws:eks:eu-central-1:111122223333:cluster/prod-eks"

// Edge case 1: two profiles into one account seeing one cluster must yield a
// single target whose ID is still the bare ARN — the whole reason labels,
// collections, hidden state and selection survive the upgrade.
func TestFoldCollapsesSharedCluster(t *testing.T) {
	in := []domain.Target{
		foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse),
		foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse),
	}
	got := foldTargetsByID(in)
	if len(got) != 1 {
		t.Fatalf("got %d targets, want exactly 1", len(got))
	}
	if string(got[0].ID) != sharedARN {
		t.Errorf("ID = %q, want the bare ARN %q", got[0].ID, sharedARN)
	}
	want := []domain.CredentialID{credentialID("dev"), credentialID("ops")}
	if !reflect.DeepEqual(got[0].CredentialIDs, want) {
		t.Errorf("CredentialIDs = %v, want %v (both healthy: alphabetical)", got[0].CredentialIDs, want)
	}
	if got[0].CredentialID != want[0] {
		t.Errorf("CredentialID = %q, want it to equal CredentialIDs[0] = %q", got[0].CredentialID, want[0])
	}
}

// Edge case 3: health ranking, not profile ordering, picks the primary. The
// alphabetically-first profile is the expired one here, so a fold that sorted
// by name would report the cluster as unreachable when it is not.
func TestFoldPrimaryIsHealthiestNotFirst(t *testing.T) {
	in := []domain.Target{
		foldCandidate("dev", sharedARN, domain.HealthExpired, domain.ActionRenew),
		foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse),
	}
	got := foldTargetsByID(in)
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Health != domain.HealthValid {
		t.Errorf("Health = %q, want %q — the working profile must win", got[0].Health, domain.HealthValid)
	}
	if got[0].CredentialID != credentialID("ops") {
		t.Errorf("CredentialID = %q, want the healthy profile's", got[0].CredentialID)
	}
	if got[0].CredentialIDs[0] != credentialID("ops") {
		t.Errorf("CredentialIDs[0] = %q, want the primary first", got[0].CredentialIDs[0])
	}
}

// Edge case 4: when no profile works the target must still name one credential
// unambiguously, so `credential renew` has a subject.
func TestFoldAllExpiredKeepsRenewSubject(t *testing.T) {
	in := []domain.Target{
		foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew),
		foldCandidate("dev", sharedARN, domain.HealthExpired, domain.ActionRenew),
	}
	got := foldTargetsByID(in)
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Health != domain.HealthExpired || got[0].ActionHint != domain.ActionRenew {
		t.Errorf("health/action = %q/%q, want expired/renew", got[0].Health, got[0].ActionHint)
	}
	if got[0].CredentialID != credentialID("dev") {
		t.Errorf("CredentialID = %q, want the alphabetical tie-break %q", got[0].CredentialID, credentialID("dev"))
	}
}

// D4, the Gate 3.5 finding: the fold must return the winning candidate's own
// struct, not scalar fields patched onto some other candidate. SystemLabels is
// where that goes wrong invisibly — SelectionLabels() exposes health under both
// the bare `health` key (from Target.Health) and `kuberoutectl.io/health` (from
// the map), so a patched target matches one selector and not the other.
//
// Compared as a whole map on purpose: a field-by-field check cannot see this.
func TestFoldKeepsPrimarySystemLabelsIntact(t *testing.T) {
	primary := foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse)
	in := []domain.Target{
		foldCandidate("dev", sharedARN, domain.HealthExpired, domain.ActionRenew),
		primary,
	}
	got := foldTargetsByID(in)[0]

	if !reflect.DeepEqual(got.SystemLabels, primary.SystemLabels) {
		t.Errorf("SystemLabels = %v, want the primary's %v", got.SystemLabels, primary.SystemLabels)
	}
	if !reflect.DeepEqual(got.Metadata, primary.Metadata) {
		t.Errorf("Metadata = %v, want the primary's %v", got.Metadata, primary.Metadata)
	}
	// The two health views a selector can reach must agree with each other.
	labels := got.SelectionLabels()
	if labels["health"] != labels[domain.LabelHealth] {
		t.Errorf("selector sees health=%q but %s=%q — the fold left the label stale",
			labels["health"], domain.LabelHealth, labels[domain.LabelHealth])
	}
	if labels[domain.LabelCredential] != "ops" {
		t.Errorf("%s = %q, want the primary profile %q", domain.LabelCredential, labels[domain.LabelCredential], "ops")
	}
}

// Edge case 2: different ARNs are different clusters and must never fold, even
// when discovered through the same profile — this is what makes the
// region-split case (each profile configured for its own region) work.
func TestFoldKeepsDistinctClustersApart(t *testing.T) {
	other := "arn:aws:eks:us-east-1:111122223333:cluster/staging-eks"
	in := []domain.Target{
		foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse),
		foldCandidate("ops", other, domain.HealthValid, domain.ActionUse),
	}
	if got := foldTargetsByID(in); len(got) != 2 {
		t.Fatalf("got %d targets, want 2 — distinct ARNs must not fold", len(got))
	}
}

// A single-credential target must come out with CredentialIDs nil, so
// providers with one way in serialize exactly as they did before this existed.
func TestFoldLeavesSingleCredentialTargetsAlone(t *testing.T) {
	in := []domain.Target{foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse)}
	got := foldTargetsByID(in)
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].CredentialIDs != nil {
		t.Errorf("CredentialIDs = %v, want nil for a single-credential target", got[0].CredentialIDs)
	}
}

// Edge case 9: the fold must not depend on the order discovery happened to
// visit profiles in, or `-o json` output would churn between syncs.
func TestFoldIsOrderIndependent(t *testing.T) {
	a := foldCandidate("dev", sharedARN, domain.HealthExpired, domain.ActionRenew)
	b := foldCandidate("ops", sharedARN, domain.HealthValid, domain.ActionUse)
	c := foldCandidate("qa", sharedARN, domain.HealthValid, domain.ActionUse)

	forward := foldTargetsByID([]domain.Target{a, b, c})
	reverse := foldTargetsByID([]domain.Target{c, b, a})
	if !reflect.DeepEqual(forward, reverse) {
		t.Errorf("fold is order-dependent:\n forward=%+v\n reverse=%+v", forward, reverse)
	}
	want := []domain.CredentialID{credentialID("ops"), credentialID("qa"), credentialID("dev")}
	if !reflect.DeepEqual(forward[0].CredentialIDs, want) {
		t.Errorf("CredentialIDs = %v, want %v (healthy alphabetical, then the expired one)",
			forward[0].CredentialIDs, want)
	}
}
