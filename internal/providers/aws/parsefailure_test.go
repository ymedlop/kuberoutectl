package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// A parse failure on a *successful* command is skipped, not fatal — one
// malformed cluster must not sink the whole sync, the rule gcp's discoverClusters
// states explicitly. But skipping it silently is the problem: the exec succeeded,
// so `--verbose` shows nothing either, and an aws CLI output-format change reads
// as "you have no clusters".
//
// These tests pin the diagnostic, not the resilience.

// malformedListRunner: prod-sso's `eks list-clusters` succeeds and returns
// garbage. Everything else behaves.
func malformedListRunner(t *testing.T) *execx.FakeRunner {
	t.Helper()
	r := execx.NewFakeRunner()
	r.Responses["aws configure list-profiles"] = execx.FakeResponse{Stdout: []byte("prod-sso\n")}
	r.Responses["aws sts get-caller-identity --profile prod-sso --output json"] = execx.FakeResponse{Stdout: readFixture(t, "identity-prod-sso.json")}
	r.Responses["aws configure get sso_start_url --profile prod-sso"] = execx.FakeResponse{Stdout: []byte("https://my-sso.awsapps.com/start\n")}
	r.Responses["aws configure get region --profile prod-sso"] = execx.FakeResponse{Stdout: []byte("eu-central-1\n")}
	r.Responses["aws eks list-clusters --profile prod-sso --region eu-central-1 --output json"] = execx.FakeResponse{Stdout: []byte("not json at all")}
	return r
}

func TestDiscover_MalformedClusterListIsReported(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, malformedListRunner(t))
	prog := &testProgress{}

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog})
	if err != nil {
		t.Fatalf("Discover must stay resilient, got: %v", err)
	}
	if len(res.Targets) != 0 {
		t.Errorf("got %d targets from unparseable output, want 0", len(res.Targets))
	}
	if len(res.Credentials) != 1 {
		t.Errorf("got %d credentials, want the profile still recorded", len(res.Credentials))
	}
	assertStepMentions(t, prog, "parse", "prod-sso")
}

// malformedDescribeRunner: the listing is fine and names two clusters, but one
// of them describes into garbage. The other must still be discovered.
func malformedDescribeRunner(t *testing.T) *execx.FakeRunner {
	t.Helper()
	r := malformedListRunner(t)
	r.Responses["aws eks list-clusters --profile prod-sso --region eu-central-1 --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-list-prod.json")}
	r.Responses["aws eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-frankfurt --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-describe-frankfurt.json")}
	r.Responses["aws eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-ireland --output json"] = execx.FakeResponse{Stdout: []byte(`{"cluster": tru`)}
	return r
}

func TestDiscover_MalformedClusterDescriptionIsReported(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, malformedDescribeRunner(t))
	prog := &testProgress{}

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog})
	if err != nil {
		t.Fatalf("Discover must stay resilient, got: %v", err)
	}
	if len(res.Targets) != 1 || res.Targets[0].Name != "eks-prod-frankfurt" {
		t.Fatalf("the healthy cluster must survive; got %d targets: %+v", len(res.Targets), res.Targets)
	}
	assertStepMentions(t, prog, "parse", "eks-prod-ireland")
}

// A malformed describe must not be reported the same way as an access denial:
// one is a format regression to investigate, the other is normal in a fleet with
// uneven permissions. Conflating them buries the rare one in the common one.
func TestDiscover_ParseFailureIsNotWordedAsAccessDenial(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, malformedDescribeRunner(t))
	prog := &testProgress{}
	if _, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, s := range prog.steps {
		if strings.Contains(s, "eks-prod-ireland") && strings.Contains(s, "cannot describe") {
			t.Errorf("a parse failure is reported as an access denial: %q", s)
		}
	}
}

// assertStepMentions fails unless some progress step contains every substring.
func assertStepMentions(t *testing.T, prog *testProgress, want ...string) {
	t.Helper()
	for _, s := range prog.steps {
		all := true
		for _, w := range want {
			if !strings.Contains(strings.ToLower(s), strings.ToLower(w)) {
				all = false
				break
			}
		}
		if all {
			return
		}
	}
	t.Errorf("no progress step mentions all of %v; steps:\n%s", want, strings.Join(prog.steps, "\n"))
}
