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

// accessEntryCallsFor narrows to one cluster. Counting fleet-wide totals was
// only ever workable while most clusters went unchecked; now that every cluster
// with a conclusive mode is checked, a total says nothing about the cluster a
// test is actually about.
func accessEntryCallsFor(calls []string, cluster string) []string {
	var out []string
	for _, c := range accessEntryCalls(calls) {
		if strings.Contains(c, "--cluster-name "+cluster+" ") {
			out = append(out, c)
		}
	}
	return out
}

// A cluster reached by ONE profile is now checked too. The original bound
// skipped it, reasoning that with one way in there is nothing to choose — true,
// but there is still something to know: whether that one way in will be refused.
// In a fleet where every cluster has a single profile, the old rule produced no
// verdict anywhere, which was most of the value of having the check at all.
func TestDiscover_SingleProfileClusterIsCheckedToo(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	var found bool
	for _, c := range accessEntryCalls(r.Calls) {
		if strings.Contains(c, "eks-prod-ireland") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no access-entry call for the single-profile cluster; calls:\n%s",
			strings.Join(accessEntryCalls(r.Calls), "\n"))
	}

	ireland := targetByName(t, res.Targets, "eks-prod-ireland")
	if ireland.AccessCheck != domain.AccessCheckAPIAndConfigMap {
		t.Errorf("AccessCheck = %q, want %q", ireland.AccessCheck, domain.AccessCheckAPIAndConfigMap)
	}
	if got := ireland.CredentialAccess(credentialID("ops")); got != domain.AccessOperable {
		t.Errorf("ops = %q, want %q — it holds an entry on this cluster", got, domain.AccessOperable)
	}
	// The whole point of the split in foldGroup: a verdict, and still no
	// credential_ids, so the no-migration property survives.
	if ireland.CredentialIDs != nil {
		t.Errorf("CredentialIDs = %v, want nil — one profile reaches this cluster", ireland.CredentialIDs)
	}
}

// A cluster whose authentication mode cannot be read is still not checked: the
// mode decides whether an absence means anything, and guessing is the one thing
// that turns silence into a confident refusal.
func TestDiscover_UnreadableModeStillMakesNoCall(t *testing.T) {
	p, r := newNoPatternAWSProviderWithRunner(t)
	for _, profile := range []string{"ops", "prod-sso"} {
		r.Responses["aws eks describe-cluster --profile "+profile+" --region eu-central-1 --name eks-prod-frankfurt --output json"] =
			execx.FakeResponse{Stdout: []byte(`{"cluster":{"name":"eks-prod-frankfurt","arn":"arn:aws:eks:eu-central-1:111111111111:cluster/eks-prod-frankfurt","status":"ACTIVE"}}`)}
	}
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, c := range accessEntryCalls(r.Calls) {
		if strings.Contains(c, "eks-prod-frankfurt") {
			t.Errorf("checked a cluster whose mode could not be read: %q", c)
		}
	}
	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	if frankfurt.AccessCheck != "" {
		t.Errorf("AccessCheck = %q, want empty", frankfurt.AccessCheck)
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
	if got := accessEntryCallsFor(r.Calls, "eks-prod-frankfurt"); len(got) != 2 {
		t.Fatalf("made %d access-entry calls for frankfurt, want 2 (one per page): %v", len(got), got)
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
	if got := accessEntryCallsFor(r.Calls, "eks-prod-frankfurt"); len(got) != 0 {
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
	if got := accessEntryCallsFor(r.Calls, "eks-prod-frankfurt"); len(got) != 0 {
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
