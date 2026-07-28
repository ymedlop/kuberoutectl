package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

func stepMentioning(steps []string, substr string) bool {
	for _, s := range steps {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func accessEntryCalls(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.Contains(c, "eks list-access-entries") {
			out = append(out, c)
		}
	}
	return out
}

// Edge case 1, the cost bound: a cluster only one profile reaches has nothing to
// disambiguate, so it must trigger no extra API call at all. Asserted on the
// recorded calls rather than on the result, because a result-level assertion
// passes just as well when the call was made and its answer discarded.
func TestDiscover_NoAccessEntryCallForSingleProfileCluster(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	if _, err := p.Discover(context.Background(), providers.DiscoveryInput{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, c := range accessEntryCalls(r.Calls) {
		if strings.Contains(c, "eks-prod-ireland") {
			t.Errorf("access entries were listed for a single-profile cluster: %q", c)
		}
	}
	if len(accessEntryCalls(r.Calls)) == 0 {
		t.Fatal("no access-entry call at all; the assertion above would pass vacuously")
	}
}

// Edge case 8: the admitted principal sits on the second page. A check that
// stops at the first page reports a real access entry as absent — which under
// `API` mode is not "we don't know", it is a confident "you will be refused".
func TestDiscover_FollowsAccessEntryPagination(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := len(accessEntryCalls(r.Calls)); got != 2 {
		t.Fatalf("made %d access-entry calls, want 2 (one per page): %v", got, accessEntryCalls(r.Calls))
	}

	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	if frankfurt.AccessCheck != domain.AccessCheckAPI {
		t.Fatalf("AccessCheck = %q, want %q", frankfurt.AccessCheck, domain.AccessCheckAPI)
	}
	if got := frankfurt.CredentialAccess(credentialID("ops")); got != domain.AccessOperable {
		t.Errorf("ops = %q, want %q — its entry is on page 2", got, domain.AccessOperable)
	}
	if got := frankfurt.CredentialAccess(credentialID("prod-sso")); got != domain.AccessNotOperable {
		t.Errorf("prod-sso = %q, want %q — absent under api mode", got, domain.AccessNotOperable)
	}
}

// A cluster nobody had to disambiguate must not gain the fields at all, so
// snapshots and `-o json` output stay as they were for single-profile fleets.
func TestDiscover_SingleProfileClusterCarriesNoAccessData(t *testing.T) {
	res, err := newNoPatternAWSProvider(t).Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	ireland := targetByName(t, res.Targets, "eks-prod-ireland")
	if ireland.AccessCheck != "" || ireland.OperableCredentialIDs != nil {
		t.Errorf("unchecked target carries access data: check=%q operable=%v",
			ireland.AccessCheck, ireland.OperableCredentialIDs)
	}
}

// Edge case 9: no eks:ListAccessEntries permission is a command failure, so it
// is resilient — the sync completes, the verdict becomes unavailable, and every
// profile reads unknown. The step must name the permission, because otherwise
// the operator sees a fleet full of `unknown` with nothing to act on.
func TestDiscover_AccessEntriesDeniedIsUnavailableNotRefused(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for key := range r.Responses {
		if strings.Contains(key, "eks list-access-entries") {
			r.Responses[key] = execx.FakeResponse{Err: failErr{}}
		}
	}
	prog := &testProgress{}
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog})
	if err != nil {
		t.Fatalf("a denied access-entry call must not fail the sync, got: %v", err)
	}

	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	if frankfurt.AccessCheck != domain.AccessCheckUnavailable {
		t.Errorf("AccessCheck = %q, want %q", frankfurt.AccessCheck, domain.AccessCheckUnavailable)
	}
	for _, id := range frankfurt.CredentialIDs {
		if got := frankfurt.CredentialAccess(id); got != domain.AccessUnknown {
			t.Errorf("CredentialAccess(%q) = %q, want %q — a failed check establishes nothing", id, got, domain.AccessUnknown)
		}
	}
	if !stepMentioning(prog.steps, "eks:ListAccessEntries") {
		t.Errorf("no step names the missing permission; steps:\n%s", strings.Join(prog.steps, "\n"))
	}
}

// Edge case 10: the command succeeded, so an unreadable body is a format
// regression and a hard error — never a quiet "nobody has access". This is the
// one place in the AWS adapter where a per-item skip would be actively
// dangerous, because the skipped answer is itself an authorization claim.
func TestDiscover_MalformedAccessEntriesIsHardError(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for key := range r.Responses {
		if strings.Contains(key, "eks list-access-entries") {
			r.Responses[key] = execx.FakeResponse{Stdout: []byte("{not json")}
		}
	}
	_, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err == nil {
		t.Fatal("want a hard error for an unparseable access-entry response, got nil")
	}
	if !strings.Contains(err.Error(), "eks-prod-frankfurt") {
		t.Errorf("error does not name the cluster: %v", err)
	}
}

// Edge case 2: a CONFIG_MAP cluster has no access entries by definition, so the
// call is skipped entirely — and the verdict must read unknown, never "not
// operable". Clusters made through the API, the SDKs or CloudFormation default
// to this mode, so this is the common path, not a corner.
func TestDiscover_ConfigMapClusterSkipsTheCallAndStaysUnknown(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for _, profile := range []string{"ops", "prod-sso"} {
		r.Responses["aws eks describe-cluster --profile "+profile+" --region eu-central-1 --name eks-prod-frankfurt --output json"] =
			execx.FakeResponse{Stdout: []byte(`{"cluster":{"name":"eks-prod-frankfurt","arn":"arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt","status":"ACTIVE","accessConfig":{"authenticationMode":"CONFIG_MAP"}}}`)}
	}
	prog := &testProgress{}
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := accessEntryCalls(r.Calls); len(got) != 0 {
		t.Errorf("made %d access-entry calls for a CONFIG_MAP cluster, want 0: %v", len(got), got)
	}

	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	if frankfurt.AccessCheck != domain.AccessCheckConfigMap {
		t.Errorf("AccessCheck = %q, want %q", frankfurt.AccessCheck, domain.AccessCheckConfigMap)
	}
	for _, id := range frankfurt.CredentialIDs {
		if got := frankfurt.CredentialAccess(id); got != domain.AccessUnknown {
			t.Errorf("CredentialAccess(%q) = %q, want %q", id, got, domain.AccessUnknown)
		}
	}
	if !stepMentioning(prog.steps, "CONFIG_MAP") {
		t.Errorf("no step explains why the cluster was not checked; steps:\n%s", strings.Join(prog.steps, "\n"))
	}
}

// Edge case 3: under API_AND_CONFIG_MAP a listed principal is still confirmed,
// but an absent one is unknown — aws-auth may grant it and kuberoutectl does not
// read aws-auth. Reporting a negative here would be a fabrication.
func TestDiscover_APIAndConfigMapNeverReportsARefusal(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for _, profile := range []string{"ops", "prod-sso"} {
		r.Responses["aws eks describe-cluster --profile "+profile+" --region eu-central-1 --name eks-prod-frankfurt --output json"] =
			execx.FakeResponse{Stdout: []byte(`{"cluster":{"name":"eks-prod-frankfurt","arn":"arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt","status":"ACTIVE","accessConfig":{"authenticationMode":"API_AND_CONFIG_MAP"}}}`)}
	}
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	if got := frankfurt.CredentialAccess(credentialID("ops")); got != domain.AccessOperable {
		t.Errorf("ops = %q, want %q — presence is trustworthy under every mode", got, domain.AccessOperable)
	}
	if got := frankfurt.CredentialAccess(credentialID("prod-sso")); got != domain.AccessUnknown {
		t.Errorf("prod-sso = %q, want %q — absence proves nothing under this mode", got, domain.AccessUnknown)
	}
}

// A cluster whose mode this build does not recognise must not be checked at all.
// Guessing would mean guessing whether absence is authoritative, and only one
// of the two possible guesses is safe.
func TestDiscover_UnknownAuthenticationModeIsNotChecked(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for _, profile := range []string{"ops", "prod-sso"} {
		r.Responses["aws eks describe-cluster --profile "+profile+" --region eu-central-1 --name eks-prod-frankfurt --output json"] =
			execx.FakeResponse{Stdout: []byte(`{"cluster":{"name":"eks-prod-frankfurt","arn":"arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt","status":"ACTIVE","accessConfig":{"authenticationMode":"SOMETHING_NEW"}}}`)}
	}
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := accessEntryCalls(r.Calls); len(got) != 0 {
		t.Errorf("checked a cluster whose mode is unrecognised: %v", got)
	}
	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	for _, id := range frankfurt.CredentialIDs {
		if got := frankfurt.CredentialAccess(id); got != domain.AccessUnknown {
			t.Errorf("CredentialAccess(%q) = %q, want %q", id, got, domain.AccessUnknown)
		}
	}
}

// Edge case 14: a profile whose STS identity carries no ARN has nothing to match
// on. It must come out unknown — never operable (an empty key matching a
// malformed entry) and never refused (a missing key read as "absent from the
// list"). Both failure modes are silent, which is why this is asserted on the
// matcher directly.
func TestMatchOperable_EmptyKeysNeverMatch(t *testing.T) {
	entries := []string{
		"arn:aws:iam::111122223333:role/PlatformAdmin",
		"not-an-arn-at-all", // also reduces to an empty key
	}
	got := matchOperable(entries, map[domain.CredentialID]string{
		credentialID("broken"): "",
		credentialID("ops"):    "111122223333/PlatformAdmin",
	})
	if got[credentialID("broken")] {
		t.Error("a credential with no usable identity was reported as admitted")
	}
	if !got[credentialID("ops")] {
		t.Error("the matching credential was not found")
	}
}
