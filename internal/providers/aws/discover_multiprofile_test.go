package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// newNoPatternAWSProvider models the environment this feature exists for: two
// SSO profiles into the SAME account, where neither has access to everything
// and there is no pattern to predict which reaches what.
//
//	prod-sso: describes frankfurt, DENIED on ireland
//	ops:      describes both
//
// Both profiles list both clusters, because eks:ListClusters is account-wide;
// it is eks:DescribeCluster that IAM evaluates per resource. That asymmetry is
// what makes reachability discoverable at all.
func newNoPatternAWSProvider(t *testing.T) *Provider {
	t.Helper()
	r := execx.NewFakeRunner()
	const ssoURL = "https://my-sso.awsapps.com/start"

	r.Responses["aws configure list-profiles"] = execx.FakeResponse{Stdout: []byte("ops\nprod-sso\n")}

	for _, profile := range []string{"prod-sso", "ops"} {
		identity := "identity-prod-sso.json"
		if profile == "ops" {
			identity = "identity-ops.json"
		}
		r.Responses["aws sts get-caller-identity --profile "+profile+" --output json"] = execx.FakeResponse{Stdout: readFixture(t, identity)}
		r.Responses["aws configure get sso_start_url --profile "+profile] = execx.FakeResponse{Stdout: []byte(ssoURL + "\n")}
		r.Responses["aws configure get region --profile "+profile] = execx.FakeResponse{Stdout: []byte("eu-central-1\n")}
		r.Responses["aws eks list-clusters --profile "+profile+" --region eu-central-1 --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-list-prod.json")}
		r.Responses["aws eks describe-cluster --profile "+profile+" --region eu-central-1 --name eks-prod-frankfurt --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-describe-frankfurt.json")}
	}
	// Only ops can describe ireland; prod-sso is denied at the IAM layer.
	r.Responses["aws eks describe-cluster --profile ops --region eu-central-1 --name eks-prod-ireland --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-describe-ireland.json")}
	r.Responses["aws eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-ireland --output json"] = execx.FakeResponse{Err: failErr{}}

	return New(fakeResolver{path: "aws"}, r)
}

func targetByName(t *testing.T, targets []domain.Target, name string) domain.Target {
	t.Helper()
	for _, tg := range targets {
		if tg.Name == name {
			return tg
		}
	}
	t.Fatalf("no target named %q in %d targets", name, len(targets))
	return domain.Target{}
}

// The shared cluster must appear exactly once, not once per profile — the bug
// this feature fixes. Asserting an exact count, not "contains", because the
// duplicates carry an identical ID and would otherwise pass a contains check.
func TestDiscover_SharedClusterFoldsToOneTarget(t *testing.T) {
	res, err := newNoPatternAWSProvider(t).Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Targets) != 2 {
		var got []string
		for _, tg := range res.Targets {
			got = append(got, string(tg.ID))
		}
		t.Fatalf("got %d targets, want 2 (frankfurt seen by both profiles, ireland by one): %v", len(res.Targets), got)
	}

	frankfurt := targetByName(t, res.Targets, "eks-prod-frankfurt")
	want := []domain.CredentialID{credentialID("ops"), credentialID("prod-sso")}
	if len(frankfurt.CredentialIDs) != 2 ||
		frankfurt.CredentialIDs[0] != want[0] || frankfurt.CredentialIDs[1] != want[1] {
		t.Errorf("frankfurt CredentialIDs = %v, want %v", frankfurt.CredentialIDs, want)
	}
	if frankfurt.CredentialID != frankfurt.CredentialIDs[0] {
		t.Errorf("CredentialID = %q, want CredentialIDs[0] = %q", frankfurt.CredentialID, frankfurt.CredentialIDs[0])
	}
}

// The no-pattern payoff: a cluster only one profile can describe records only
// that profile, so `target use` with no flag picks a working way in without
// trying and failing against the cluster.
func TestDiscover_ClusterReachableByOneProfileRecordsOnlyIt(t *testing.T) {
	res, err := newNoPatternAWSProvider(t).Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	ireland := targetByName(t, res.Targets, "eks-prod-ireland")
	if ireland.CredentialIDs != nil {
		t.Errorf("CredentialIDs = %v, want nil — only one profile reaches this cluster", ireland.CredentialIDs)
	}
	if ireland.CredentialID != credentialID("ops") {
		t.Errorf("CredentialID = %q, want %q — the only profile that can describe it",
			ireland.CredentialID, credentialID("ops"))
	}
	if ireland.Metadata["profile"] != "ops" {
		t.Errorf("metadata profile = %q, want ops", ireland.Metadata["profile"])
	}
}

// Task 4b: a per-cluster access denial used to be a silent `continue`. In an
// environment with no documented access map, that step output IS the map.
func TestDiscover_ReportsPerClusterAccessDenial(t *testing.T) {
	prog := &testProgress{}
	if _, err := newNoPatternAWSProvider(t).Discover(context.Background(), providers.DiscoveryInput{Progress: prog}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var found string
	for _, s := range prog.steps {
		if strings.Contains(s, "eks-prod-ireland") && strings.Contains(s, "prod-sso") {
			found = s
			break
		}
	}
	if found == "" {
		t.Fatalf("no step named both the denied profile and cluster; steps:\n%s", strings.Join(prog.steps, "\n"))
	}
	// The profile that succeeded must not be reported as denied for it.
	for _, s := range prog.steps {
		if strings.Contains(s, "eks-prod-ireland") && strings.Contains(s, "\"ops\"") {
			t.Errorf("ops can describe ireland but was reported as denied: %q", s)
		}
	}
}

// Discovery stays resilient: a whole profile failing STS contributes no
// targets and therefore appears in no target's CredentialIDs, while still
// yielding a credential so the operator sees what needs attention.
func TestDiscover_FailedProfileIsInNoCredentialIDs(t *testing.T) {
	p, _ := newFakeAWSProvider(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var sawFailedCredential bool
	for _, c := range res.Credentials {
		if c.ID == credentialID("default") {
			sawFailedCredential = true
		}
	}
	if !sawFailedCredential {
		t.Error("the STS-failed profile must still yield a credential")
	}
	for _, tg := range res.Targets {
		for _, id := range append(tg.CredentialIDs, tg.CredentialID) {
			if id == credentialID("default") {
				t.Errorf("target %q lists the STS-failed profile as an access path", tg.ID)
			}
		}
	}
}
