package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// The provider must satisfy the optional interface, or every caller silently
// falls through the type assertion and the feature does nothing.
var _ providers.AccessChecker = (*Provider)(nil)

// liveTarget is a cluster as the snapshot holds it, with the authentication mode
// discovery recorded in provider metadata.
func liveTarget(mode string) domain.Target {
	return domain.Target{
		ID: "arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt", ProviderID: ProviderID,
		Name: "eks-prod-frankfurt", Region: "eu-central-1",
		CredentialID: credentialID("ops"),
		Metadata:     map[string]string{"profile": "ops", "authentication_mode": mode},
	}
}

// The credentials as discovery stored them: Identity is the STS ARN, which is
// what makes a live check cost one call rather than one plus an STS per profile.
func liveCreds() []domain.Credential {
	return []domain.Credential{
		{ID: credentialID("ops"), Name: "ops", Identity: "arn:aws:sts::111111111111:assumed-role/AWSReservedSSO_BreakGlass/yeray"},
		{ID: credentialID("prod-sso"), Name: "prod-sso", Identity: "arn:aws:sts::111111111111:assumed-role/AWSReservedSSO_Platform/yeray"},
	}
}

func liveProvider(t *testing.T) (*Provider, *execx.FakeRunner) {
	t.Helper()
	r := execx.NewFakeRunner()
	r.Responses["aws eks list-access-entries --cluster-name eks-prod-frankfurt --profile ops --region eu-central-1 --output json"] =
		execx.FakeResponse{Stdout: readFixture(t, "access-entries-page1.json")}
	r.Responses["aws eks list-access-entries --cluster-name eks-prod-frankfurt --profile ops --region eu-central-1 --output json --starting-token eyJwYWdlIjogMn0="] =
		execx.FakeResponse{Stdout: readFixture(t, "access-entries-page2.json")}
	return New(fakeResolver{path: "aws"}, r), r
}

// One call answers for every credential — the whole reason the interface takes
// the set rather than one. An implementation that looped per credential would
// satisfy every other assertion in this file.
func TestCheckAccess_OneCallForEveryCredential(t *testing.T) {
	p, r := liveProvider(t)

	got, err := p.CheckAccess(context.Background(), liveTarget("API"), liveCreds())
	if err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	// Two calls, both pagination pages of a single logical lookup.
	if n := len(accessEntryCalls(r.Calls)); n != 2 {
		t.Errorf("made %d access-entry calls for 2 credentials, want 2 (one lookup, two pages): %v",
			n, accessEntryCalls(r.Calls))
	}
	if got.Mode != domain.AccessCheckAPI {
		t.Errorf("Mode = %q, want %q", got.Mode, domain.AccessCheckAPI)
	}
	if len(got.Operable) != 1 || got.Operable[0] != credentialID("ops") {
		t.Errorf("Operable = %v, want just ops (its entry is on page 2)", got.Operable)
	}
	if got.Reason != "" {
		t.Errorf("Reason = %q, want empty on a conclusive answer", got.Reason)
	}
}

// The verdict derivation is not re-tested here — it is domain's table. What is
// tested is that the facts feeding it are right in both directions.
func TestCheckAccess_VerdictsDeriveCorrectly(t *testing.T) {
	p, _ := liveProvider(t)
	got, err := p.CheckAccess(context.Background(), liveTarget("API"), liveCreds())
	if err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	tgt := liveTarget("API")
	tgt.AccessCheck, tgt.OperableCredentialIDs = got.Mode, got.Operable

	if v := tgt.CredentialAccess(credentialID("ops")); v != domain.AccessOperable {
		t.Errorf("ops = %q, want %q", v, domain.AccessOperable)
	}
	if v := tgt.CredentialAccess(credentialID("prod-sso")); v != domain.AccessNotOperable {
		t.Errorf("prod-sso = %q, want %q under api mode", v, domain.AccessNotOperable)
	}
}

// A CONFIG_MAP cluster costs nothing: the mode came back with describe-cluster,
// and entries do not apply.
func TestCheckAccess_ConfigMapMakesNoCall(t *testing.T) {
	p, r := liveProvider(t)

	got, err := p.CheckAccess(context.Background(), liveTarget("CONFIG_MAP"), liveCreds())
	if err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	if n := len(accessEntryCalls(r.Calls)); n != 0 {
		t.Errorf("made %d calls for a CONFIG_MAP cluster, want 0", n)
	}
	// Mode without Reason: the check ran, and "entries do not apply" is the
	// answer. Reason is reserved for a check that could not run, so setting it
	// here made a conclusive read indistinguishable from a failure downstream.
	if got.Mode != domain.AccessCheckConfigMap {
		t.Errorf("Mode = %q, want %q", got.Mode, domain.AccessCheckConfigMap)
	}
	if len(got.Operable) != 0 {
		t.Errorf("Operable = %v, want none — no entries exist under this mode", got.Operable)
	}
	if got.Reason != "" {
		t.Errorf("Reason = %q, want empty: the check ran and concluded", got.Reason)
	}
}

// The contract this feature turns on: a runtime failure is data, never an error.
// The natural implementation propagates — parseAccessEntries returns a hard
// error by design and UseTarget propagates every err it sees — and that would
// block activations.
func TestCheckAccess_NeverErrorsOnARuntimeFailure(t *testing.T) {
	cases := []struct {
		name        string
		response    execx.FakeResponse
		wantMode    domain.AccessCheckMode
		reasonHas   string
		reasonHasNo string
	}{
		{
			name:      "the call was denied",
			response:  execx.FakeResponse{Err: failErr{}},
			wantMode:  domain.AccessCheckUnavailable,
			reasonHas: "eks:ListAccessEntries",
		},
		{
			// Must be distinguishable from every routine "nothing to tell": a format
			// regression phrased like a CONFIG_MAP cluster hides behind the common
			// case indefinitely.
			name:      "the response could not be parsed",
			response:  execx.FakeResponse{Stdout: []byte("{not json")},
			wantMode:  domain.AccessCheckUnavailable,
			reasonHas: "format change",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, r := liveProvider(t)
			for key := range r.Responses {
				r.Responses[key] = tc.response
			}
			got, err := p.CheckAccess(context.Background(), liveTarget("API"), liveCreds())
			if err != nil {
				t.Fatalf("a runtime failure must be data, not an error: %v", err)
			}
			if got.Mode != tc.wantMode {
				t.Errorf("Mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if !strings.Contains(got.Reason, tc.reasonHas) {
				t.Errorf("Reason = %q, want it to mention %q", got.Reason, tc.reasonHas)
			}
			if len(got.Operable) != 0 {
				t.Errorf("Operable = %v, want none when nothing was established", got.Operable)
			}
		})
	}
}

// The two failure reasons must not read alike, which is the whole point of
// distinguishing them. Asserted by comparing them, because each one looked
// reasonable in isolation above.
func TestCheckAccess_FormatRegressionReadsDifferentlyFromRoutineSilence(t *testing.T) {
	p, r := liveProvider(t)
	for key := range r.Responses {
		r.Responses[key] = execx.FakeResponse{Stdout: []byte("{not json")}
	}
	malformed, _ := p.CheckAccess(context.Background(), liveTarget("API"), liveCreds())

	// The routine comparison is now a mode that could not be read at all, since
	// CONFIG_MAP stopped carrying a Reason: both of these are "could not run",
	// and only one of them is a bug.
	p2, _ := liveProvider(t)
	routine, _ := p2.CheckAccess(context.Background(), liveTarget(""), liveCreds())

	if malformed.Reason == routine.Reason {
		t.Fatalf("a format regression and an unreadable mode read identically: %q", routine.Reason)
	}
	if strings.Contains(routine.Reason, "format change") {
		t.Errorf("a routine explanation must not cry format change: %q", routine.Reason)
	}
}

// A target synced before the mode was recorded cannot be checked, and the
// remedy is specific: a resync, not a permission.
func TestCheckAccess_UnknownModeNamesTheRemedy(t *testing.T) {
	p, r := liveProvider(t)

	got, err := p.CheckAccess(context.Background(), liveTarget(""), liveCreds())
	if err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	if n := len(accessEntryCalls(r.Calls)); n != 0 {
		t.Errorf("made %d calls for an unreadable mode, want 0", n)
	}
	if !strings.Contains(got.Reason, "sync") {
		t.Errorf("Reason = %q, want it to name the resync that fixes it", got.Reason)
	}
}

// A credential whose identity never resolved cannot be matched, and must come
// out unknown alone rather than blanking the whole result.
func TestCheckAccess_OneUnusableCredentialDoesNotSinkTheRest(t *testing.T) {
	p, _ := liveProvider(t)
	creds := append(liveCreds(), domain.Credential{ID: credentialID("broken"), Name: "broken"})

	got, err := p.CheckAccess(context.Background(), liveTarget("API"), creds)
	if err != nil {
		t.Fatalf("CheckAccess: %v", err)
	}
	if len(got.Operable) != 1 || got.Operable[0] != credentialID("ops") {
		t.Errorf("Operable = %v, want just ops", got.Operable)
	}
}

// The error return is for a caller mistake only.
func TestCheckAccess_RejectsAForeignTarget(t *testing.T) {
	p, _ := liveProvider(t)
	tgt := liveTarget("API")
	tgt.ProviderID = "gcp"

	if _, err := p.CheckAccess(context.Background(), tgt, liveCreds()); err == nil {
		t.Fatal("want an error for a target this provider does not own")
	}
}
