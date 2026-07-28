package aws

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// The mode arrives on the existing describe-cluster response — no extra call.
// A cluster predating access entries has no accessConfig block at all, and that
// must decode to the empty mode rather than failing: an older cluster is a
// normal thing to find, not a parse error.
func TestParseEKSDescribe_AuthenticationMode(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "api",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"API"}}}`,
			want: "API",
		},
		{
			name: "api and config map",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"API_AND_CONFIG_MAP"}}}`,
			want: "API_AND_CONFIG_MAP",
		},
		{
			name: "config map",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"CONFIG_MAP"}}}`,
			want: "CONFIG_MAP",
		},
		{
			name: "no accessConfig block at all",
			body: `{"cluster":{"name":"c","arn":"a","status":"ACTIVE"}}`,
			want: "",
		},
		{
			name: "accessConfig present but empty",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{}}}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseEKSDescribe([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseEKSDescribe: %v", err)
			}
			if c.AccessConfig.AuthenticationMode != tc.want {
				t.Errorf("AuthenticationMode = %q, want %q", c.AccessConfig.AuthenticationMode, tc.want)
			}
		})
	}
}

// The central function of this feature, and the one most likely to be written
// wrong in a way that still looks right.
//
// An access entry names an IAM principal; STS returns an assumed-role. They are
// never equal as strings, so both are reduced to account + final role/user
// name. The extraction direction is NOT the same for the two shapes:
//
//	assumed-role/NAME/session   → the FIRST segment is the name
//	role/…path…/NAME            → the LAST segment is the name
//
// An implementation that takes the last segment in both cases passes the plain
// role and the IAM user, and fails only the SSO path — which is the one case
// this entire feature exists for. Both shapes are in this one table so that
// implementation cannot go green.
func TestPrincipalKey(t *testing.T) {
	cases := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "edge case 4a: STS assumed-role, session name discarded",
			arn:  "arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_Platform_ab12/yeray",
			want: "111122223333/AWSReservedSSO_Platform_ab12",
		},
		{
			name: "edge case 4b: the SSO entry for that same identity, IAM path discarded",
			arn:  "arn:aws:iam::111122223333:role/aws-reserved/sso.amazonaws.com/eu-central-1/AWSReservedSSO_Platform_ab12",
			want: "111122223333/AWSReservedSSO_Platform_ab12",
		},
		{
			name: "edge case 5a: plain role entry, no path",
			arn:  "arn:aws:iam::111122223333:role/PlatformAdmin",
			want: "111122223333/PlatformAdmin",
		},
		{
			name: "edge case 5b: assuming that plain role",
			arn:  "arn:aws:sts::111122223333:assumed-role/PlatformAdmin/session-1",
			want: "111122223333/PlatformAdmin",
		},
		{
			name: "edge case 6: IAM user, no session component at all",
			arn:  "arn:aws:iam::111122223333:user/ci-bot",
			want: "111122223333/ci-bot",
		},
		{
			name: "IAM user under a path",
			arn:  "arn:aws:iam::111122223333:user/automation/ci-bot",
			want: "111122223333/ci-bot",
		},
		{
			name: "a different account is a different principal",
			arn:  "arn:aws:iam::999988887777:role/PlatformAdmin",
			want: "999988887777/PlatformAdmin",
		},
		{
			name: "govcloud partition — the partition is not part of the key",
			arn:  "arn:aws-us-gov:iam::111122223333:role/PlatformAdmin",
			want: "111122223333/PlatformAdmin",
		},

		// Everything below must yield "", which callers treat as "no key" and
		// therefore "unknown". A wrong key here would match nothing, which under
		// api mode reads as a confirmed refusal — so failing closed means
		// returning nothing, not guessing.
		{name: "empty string", arn: "", want: ""},
		{name: "not an ARN at all", arn: "yeray", want: ""},
		{name: "too few colons", arn: "arn:aws:iam::111122223333", want: ""},
		{name: "no resource", arn: "arn:aws:iam::111122223333:", want: ""},
		{name: "resource type with no name", arn: "arn:aws:iam::111122223333:role/", want: ""},
		{name: "root user has no name segment", arn: "arn:aws:iam::111122223333:root", want: ""},
		{name: "empty account", arn: "arn:aws:iam:::role/PlatformAdmin", want: ""},
		{name: "a resource type we do not match on", arn: "arn:aws:sts::111122223333:federated-user/dev", want: ""},
		{name: "assumed-role with no session", arn: "arn:aws:sts::111122223333:assumed-role/PlatformAdmin", want: "111122223333/PlatformAdmin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := principalKey(tc.arn); got != tc.want {
				t.Errorf("principalKey(%q) = %q, want %q", tc.arn, got, tc.want)
			}
		})
	}
}

// Edge case 14: a profile whose STS identity is empty has no key to match, and
// an empty key must never match an entry — otherwise a broken identity would be
// reported as operable on every cluster.
func TestPrincipalKey_EmptyNeverMatchesAnEntry(t *testing.T) {
	entries := []string{
		"arn:aws:iam::111122223333:role/PlatformAdmin",
		"arn:aws:iam::111122223333:user/ci-bot",
	}
	for _, e := range entries {
		if principalKey("") == principalKey(e) {
			t.Errorf("an empty identity matched entry %q", e)
		}
	}
}

// Edge case 10: the command succeeded, so a body we cannot read is a format
// regression, not an empty cluster. Degrading it to "nobody has access" would
// turn a kuberoutectl bug into a confident claim about someone's permissions.
func TestParseAccessEntries(t *testing.T) {
	t.Run("first page carries a nextToken", func(t *testing.T) {
		got, err := parseAccessEntries(readFixture(t, "access-entries-page1.json"))
		if err != nil {
			t.Fatalf("parseAccessEntries: %v", err)
		}
		if got.NextToken == "" {
			t.Error("NextToken is empty; the pagination test below would prove nothing")
		}
		if len(got.AccessEntries) == 0 {
			t.Fatal("no entries decoded")
		}
	})

	t.Run("last page has no token", func(t *testing.T) {
		got, err := parseAccessEntries(readFixture(t, "access-entries-page2.json"))
		if err != nil {
			t.Fatalf("parseAccessEntries: %v", err)
		}
		if got.NextToken != "" {
			t.Errorf("NextToken = %q, want empty on the last page", got.NextToken)
		}
	})

	t.Run("a cluster with no entries is not an error", func(t *testing.T) {
		got, err := parseAccessEntries([]byte(`{"accessEntries":[]}`))
		if err != nil {
			t.Fatalf("an empty list is a legitimate answer, got: %v", err)
		}
		if len(got.AccessEntries) != 0 {
			t.Errorf("entries = %v, want none", got.AccessEntries)
		}
	})

	t.Run("malformed body is a wrapped hard error", func(t *testing.T) {
		if _, err := parseAccessEntries([]byte(`{"accessEntries": [`)); err == nil {
			t.Fatal("want an error for a malformed body, got nil")
		}
	})
}

// Edge case 11, and the reason the ranking is ordered this way round: an
// expired session is a renewable obstacle (`aws sso login`, already named by
// action_hint), while a missing access entry cannot be fixed from this CLI at
// all. Ranking health first would hand the operator a healthy profile that the
// cluster refuses — the exact failure this feature exists to prevent, only now
// with data on hand that could have avoided it.
func TestFoldGroup_OperabilityOutranksHealth(t *testing.T) {
	expiredButAdmitted := foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew)
	healthyButRefused := foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse)

	got := foldGroup([]domain.Target{healthyButRefused, expiredButAdmitted}, accessResult{
		check:    domain.AccessCheckAPI,
		operable: map[domain.CredentialID]bool{credentialID("ops"): true},
	})

	if got.CredentialID != credentialID("ops") {
		t.Errorf("primary = %q, want the expired-but-admitted profile %q", got.CredentialID, credentialID("ops"))
	}
	if got.Health != domain.HealthExpired {
		t.Errorf("Health = %q, want the primary's own %q — not the healthiest candidate's", got.Health, domain.HealthExpired)
	}
	if got.CredentialIDs[0] != got.CredentialID {
		t.Errorf("CredentialIDs[0] = %q, want it to equal CredentialID %q", got.CredentialIDs[0], got.CredentialID)
	}
	if got.CredentialAccess(credentialID("ops")) != domain.AccessOperable {
		t.Error("the admitted profile must read as operable")
	}
	if got.CredentialAccess(credentialID("dev")) != domain.AccessNotOperable {
		t.Error("under api mode, an absent profile must read as not operable")
	}
}

// Without a check, ranking must be exactly what it was before this feature —
// health then name. This is what keeps every non-multi-profile cluster, and
// every cluster whose check failed, behaving as it did.
func TestFoldGroup_NoCheckFallsBackToHealthOrder(t *testing.T) {
	got := foldGroup([]domain.Target{
		foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew),
		foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse),
	}, accessResult{})

	if got.CredentialID != credentialID("dev") {
		t.Errorf("primary = %q, want the healthy profile with no check in play", got.CredentialID)
	}
	if got.AccessCheck != "" || got.OperableCredentialIDs != nil {
		t.Errorf("unchecked group carries access data: check=%q operable=%v", got.AccessCheck, got.OperableCredentialIDs)
	}
	for _, id := range got.CredentialIDs {
		if got.CredentialAccess(id) != domain.AccessUnknown {
			t.Errorf("CredentialAccess(%q) must be unknown when nothing was checked", id)
		}
	}
}

// Edge case 3, at the ranking level: under API_AND_CONFIG_MAP an absent profile
// is unknown, not refused, so it must not be demoted below a confirmed refusal
// — and with no confirmed admission in the group, health decides as usual.
func TestFoldGroup_UnknownIsNotTreatedAsRefused(t *testing.T) {
	got := foldGroup([]domain.Target{
		foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew),
		foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse),
	}, accessResult{check: domain.AccessCheckAPIAndConfigMap})

	if got.CredentialID != credentialID("dev") {
		t.Errorf("primary = %q, want the healthy one — nothing was established about either", got.CredentialID)
	}
	if got.OperableCredentialIDs != nil {
		t.Errorf("OperableCredentialIDs = %v, want none", got.OperableCredentialIDs)
	}
	for _, id := range got.CredentialIDs {
		if v := got.CredentialAccess(id); v != domain.AccessUnknown {
			t.Errorf("CredentialAccess(%q) = %q, want %q — absence proves nothing under this mode", id, v, domain.AccessUnknown)
		}
	}
}

// D4 again, now that ranking has a second key: the fold must still return the
// winner's own struct rather than scalars patched onto another candidate.
// SystemLabels is where that breaks invisibly, and the operability ranking makes
// it more likely — the winner is no longer the healthiest, so a lazy
// implementation that starts from the healthiest and patches Health across
// would leave kuberoutectl.io/health and kuberoutectl.io/credential describing a
// different profile than the target itself.
//
// The expected maps are cloned before the call: copying a Go struct copies map
// fields by reference, so comparing against the un-cloned map would report equal
// no matter what the fold wrote into it.
func TestFoldGroup_ReturnsWinnersOwnStruct(t *testing.T) {
	winner := foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew)
	wantLabels := maps.Clone(winner.SystemLabels)
	wantMetadata := maps.Clone(winner.Metadata)

	got := foldGroup([]domain.Target{
		foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse),
		winner,
	}, accessResult{
		check:    domain.AccessCheckAPI,
		operable: map[domain.CredentialID]bool{credentialID("ops"): true},
	})

	if !reflect.DeepEqual(got.SystemLabels, wantLabels) {
		t.Errorf("SystemLabels = %v, want the winner's %v", got.SystemLabels, wantLabels)
	}
	if !reflect.DeepEqual(got.Metadata, wantMetadata) {
		t.Errorf("Metadata = %v, want the winner's %v", got.Metadata, wantMetadata)
	}
	labels := got.SelectionLabels()
	if labels["health"] != labels[domain.LabelHealth] {
		t.Errorf("selector sees health=%q but %s=%q — the fold left the label stale",
			labels["health"], domain.LabelHealth, labels[domain.LabelHealth])
	}
	if labels[domain.LabelCredential] != "ops" {
		t.Errorf("%s = %q, want the winning profile %q", domain.LabelCredential, labels[domain.LabelCredential], "ops")
	}
}

// Edge case 12: when every profile is confirmed refused the target must still
// name a primary, chosen on health, so `target use` has something to act on and
// `credential renew` has a subject. Refusing to pick would strand the cluster.
func TestFoldGroup_AllRefusedStillNamesAPrimary(t *testing.T) {
	got := foldGroup([]domain.Target{
		foldCandidate("ops", sharedARN, domain.HealthExpired, domain.ActionRenew),
		foldCandidate("dev", sharedARN, domain.HealthValid, domain.ActionUse),
	}, accessResult{check: domain.AccessCheckAPI})

	if got.CredentialID != credentialID("dev") {
		t.Errorf("primary = %q, want the healthiest of a uniformly refused group", got.CredentialID)
	}
	if len(got.CredentialIDs) != 2 {
		t.Errorf("CredentialIDs = %v, want both still listed", got.CredentialIDs)
	}
	if got.OperableCredentialIDs != nil {
		t.Errorf("OperableCredentialIDs = %v, want none", got.OperableCredentialIDs)
	}
}

// The mode is a property of the cluster, identical across every profile that
// sees it, so it can ride on provider-private metadata and survive the fold's
// winner-copy unchanged. It is deliberately NOT written to Target.AccessCheck
// here: an empty AccessCheck means "not attempted", and a single-profile target
// must stay that way even though its mode is perfectly well known.
func TestBuildTarget_CarriesAuthenticationModeInMetadata(t *testing.T) {
	tgt := buildTarget("ops", "eu-central-1",
		awsIdentity{Account: "111122223333"},
		awsCluster{
			Name:         "c",
			Arn:          "arn:aws:eks:eu-central-1:111122223333:cluster/c",
			AccessConfig: awsAccessConfig{AuthenticationMode: "API"},
		},
		domain.HealthValid, domain.ActionUse, time.Unix(0, 0).UTC(),
	)
	if got := tgt.Metadata["authentication_mode"]; got != "API" {
		t.Errorf("metadata authentication_mode = %q, want %q", got, "API")
	}
	if tgt.AccessCheck != "" {
		t.Errorf("AccessCheck = %q, want empty — buildTarget must not claim a check was attempted", tgt.AccessCheck)
	}
}
