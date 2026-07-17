package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

type fakeResolver struct{ path string }

func (f fakeResolver) Resolve(string) (string, error) { return f.path, nil }

type failErr struct{}

func (failErr) Error() string { return "exit status 1" }

func TestParseProfiles(t *testing.T) {
	got := parseProfiles(readFixture(t, "list-profiles.txt"))
	want := []string{"default", "prod-sso", "legacy-static"}
	if len(got) != len(want) {
		t.Fatalf("got %d profiles, want %d: %v", len(got), len(want), got)
	}
}

func TestClassifyAuth(t *testing.T) {
	cases := []struct {
		name   string
		sso    string
		arn    string
		stsOK  bool
		expect string
	}{
		{"sso wins over arn", "https://x.awsapps.com/start", "arn:aws:sts::1:assumed-role/r/u", true, authSSO},
		{"static user", "", "arn:aws:iam::1:user/ci", true, authStatic},
		{"assumed role", "", "arn:aws:sts::1:assumed-role/r/u", true, authRole},
		{"failed no sso", "", "", false, authUnknown},
	}
	for _, c := range cases {
		if got := classifyAuth(c.sso, c.arn, c.stsOK); got != c.expect {
			t.Errorf("%s: classifyAuth = %q, want %q", c.name, got, c.expect)
		}
	}
}

func TestMapAWSHealth(t *testing.T) {
	cases := []struct {
		auth   string
		stsOK  bool
		health domain.AccessHealth
		action domain.ActionHint
	}{
		{authStatic, true, domain.HealthStatic, domain.ActionNone},
		{authSSO, true, domain.HealthValid, domain.ActionUse},
		{authRole, true, domain.HealthValid, domain.ActionUse},
		{authSSO, false, domain.HealthExpired, domain.ActionRenew},
		{authRole, false, domain.HealthExpired, domain.ActionRenew},
		{authStatic, false, domain.HealthError, domain.ActionManual},
		{authUnknown, false, domain.HealthUnknown, domain.ActionManual},
	}
	for _, c := range cases {
		h, a := mapAWSHealth(c.auth, c.stsOK)
		if h != c.health || a != c.action {
			t.Errorf("mapAWSHealth(%s,%v) = (%s,%s), want (%s,%s)", c.auth, c.stsOK, h, a, c.health, c.action)
		}
	}
}

// newFakeAWSProvider primes a FakeRunner for three profiles exercising each
// auth path: default (SSO, session expired), legacy-static (static keys, no
// clusters), prod-sso (SSO, valid, two EKS clusters).
func newFakeAWSProvider(t *testing.T) (*Provider, *execx.FakeRunner) {
	t.Helper()
	r := execx.NewFakeRunner()
	const ssoURL = "https://my-sso.awsapps.com/start"

	// default: SSO configured but STS fails (expired session).
	r.Responses["aws sts get-caller-identity --profile default --output json"] = execx.FakeResponse{Err: failErr{}}
	r.Responses["aws configure get sso_start_url --profile default"] = execx.FakeResponse{Stdout: []byte(ssoURL + "\n")}

	// legacy-static: static IAM user keys, region set, no EKS clusters.
	r.Responses["aws sts get-caller-identity --profile legacy-static --output json"] = execx.FakeResponse{Stdout: readFixture(t, "identity-static.json")}
	r.Responses["aws configure get sso_start_url --profile legacy-static"] = execx.FakeResponse{Err: failErr{}}
	r.Responses["aws configure get region --profile legacy-static"] = execx.FakeResponse{Stdout: []byte("us-east-1\n")}
	r.Responses["aws eks list-clusters --profile legacy-static --region us-east-1 --output json"] = execx.FakeResponse{Stdout: []byte(`{"clusters":[]}`)}

	// prod-sso: SSO valid, two clusters in eu-central-1.
	r.Responses["aws sts get-caller-identity --profile prod-sso --output json"] = execx.FakeResponse{Stdout: readFixture(t, "identity-prod-sso.json")}
	r.Responses["aws configure get sso_start_url --profile prod-sso"] = execx.FakeResponse{Stdout: []byte(ssoURL + "\n")}
	r.Responses["aws configure get region --profile prod-sso"] = execx.FakeResponse{Stdout: []byte("eu-central-1\n")}
	r.Responses["aws eks list-clusters --profile prod-sso --region eu-central-1 --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-list-prod.json")}
	r.Responses["aws eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-frankfurt --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-describe-frankfurt.json")}
	r.Responses["aws eks describe-cluster --profile prod-sso --region eu-central-1 --name eks-prod-ireland --output json"] = execx.FakeResponse{Stdout: readFixture(t, "eks-describe-ireland.json")}

	// list-profiles: unsorted on purpose; Discover sorts.
	r.Responses["aws configure list-profiles"] = execx.FakeResponse{Stdout: readFixture(t, "list-profiles.txt")}

	return New(fakeResolver{path: "aws"}, r), r
}

func TestDiscover_AWSFullInventory(t *testing.T) {
	p, _ := newFakeAWSProvider(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(res.Sources) != 3 {
		t.Fatalf("expected 3 sources (one per profile), got %d", len(res.Sources))
	}
	if len(res.Credentials) != 3 {
		t.Fatalf("expected 3 credentials, got %d", len(res.Credentials))
	}
	// Two accounts have usable identities -> two scopes (default's STS failed).
	if len(res.Scopes) != 2 {
		t.Fatalf("expected 2 account scopes, got %d: %+v", len(res.Scopes), res.Scopes)
	}
	// Only prod-sso has clusters.
	if len(res.Targets) != 2 {
		t.Fatalf("expected 2 EKS targets, got %d", len(res.Targets))
	}

	byName := map[string]domain.Credential{}
	for _, c := range res.Credentials {
		byName[c.Name] = c
	}
	if got := byName["default"]; got.Health != domain.HealthExpired || got.ActionHint != domain.ActionRenew {
		t.Errorf("default (SSO expired) = (%s,%s), want (expired,renew)", got.Health, got.ActionHint)
	}
	if got := byName["legacy-static"]; got.Health != domain.HealthStatic || got.ActionHint != domain.ActionNone {
		t.Errorf("legacy-static = (%s,%s), want (static,none)", got.Health, got.ActionHint)
	}
	if got := byName["prod-sso"]; got.Health != domain.HealthValid || got.ActionHint != domain.ActionUse {
		t.Errorf("prod-sso = (%s,%s), want (valid,use)", got.Health, got.ActionHint)
	}

	for _, tg := range res.Targets {
		if tg.Platform != "eks" {
			t.Errorf("target %s platform = %q, want eks", tg.Name, tg.Platform)
		}
		if tg.KubernetesVersion != "1.29" {
			t.Errorf("target %s KubernetesVersion = %q, want 1.29", tg.Name, tg.KubernetesVersion)
		}
		if tg.ScopeID != "aws:account:111111111111" {
			t.Errorf("target %s scope = %q, want prod account", tg.Name, tg.ScopeID)
		}
		if tg.SystemLabels[domain.LabelProvider] != "aws" {
			t.Errorf("target %s missing provider label", tg.Name)
		}
		if len(tg.UserLabels) != 0 {
			t.Errorf("provider must not set user labels")
		}
	}
}

func TestRenew_StaticIsRefused(t *testing.T) {
	p, _ := newFakeAWSProvider(t)
	cred := domain.Credential{ID: "aws:legacy-static", Metadata: map[string]string{"profile": "legacy-static", "auth_type": authStatic}}
	if err := p.Renew(context.Background(), cred); err == nil {
		t.Fatal("expected renew to be refused for static-key profile")
	}
}

func TestRenew_SSORunsLogin(t *testing.T) {
	p, r := newFakeAWSProvider(t)
	r.Responses["aws sso login --profile prod-sso"] = execx.FakeResponse{}
	cred := domain.Credential{ID: "aws:prod-sso", Metadata: map[string]string{"profile": "prod-sso", "auth_type": authSSO}}
	if err := p.Renew(context.Background(), cred); err != nil {
		t.Fatalf("expected SSO renew to succeed, got %v", err)
	}
	found := false
	for _, c := range r.Calls {
		if c == "aws sso login --profile prod-sso" {
			found = true
		}
	}
	if !found {
		t.Error("expected `aws sso login --profile prod-sso` to be invoked")
	}
}
