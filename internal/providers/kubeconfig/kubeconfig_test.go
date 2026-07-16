package kubeconfig

import (
	"context"
	"errors"
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

func TestClassifyUserAuth(t *testing.T) {
	cases := []struct {
		name string
		user kcUser
		want string
	}{
		{"exec wins", kcUser{Exec: &kcExec{Command: "aws"}, Token: "t"}, authExec},
		{"auth-provider", kcUser{AuthProvider: &kcAuthProvider{Name: "oidc"}}, authProviderFn},
		{"client cert data", kcUser{ClientCertificateData: "x"}, authClientCert},
		{"client cert file", kcUser{ClientCertificate: "/p.crt"}, authClientCert},
		{"token", kcUser{Token: "abc"}, authToken},
		{"token file", kcUser{TokenFile: "/var/run/token"}, authToken},
		{"basic", kcUser{Username: "admin"}, authBasic},
		{"empty", kcUser{}, authUnknown},
	}
	for _, c := range cases {
		if got := classifyUserAuth(c.user); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMapKubeconfigHealth(t *testing.T) {
	// Nothing is renewable: no auth type ever maps to renew.
	cases := map[string]domain.AccessHealth{
		authClientCert: domain.HealthStatic,
		authToken:      domain.HealthStatic,
		authBasic:      domain.HealthStatic,
		authExec:       domain.HealthUnknown,
		authProviderFn: domain.HealthUnknown,
		authUnknown:    domain.HealthUnknown,
	}
	for authType, wantHealth := range cases {
		health, action := mapKubeconfigHealth(authType)
		if health != wantHealth {
			t.Errorf("%s: health = %q, want %q", authType, health, wantHealth)
		}
		if action == domain.ActionRenew {
			t.Errorf("%s: kubeconfig credentials must never map to renew", authType)
		}
	}
}

func TestDiscover(t *testing.T) {
	runner := execx.NewFakeRunner()
	runner.Responses["kubectl config view --raw -o json"] = execx.FakeResponse{Stdout: readFixture(t, "config-view.json")}
	p := New(fakeResolver{path: "kubectl"}, runner)

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Sources) != 1 {
		t.Errorf("sources = %d, want 1", len(res.Sources))
	}
	if len(res.Scopes) != 2 {
		t.Errorf("scopes = %d, want 2", len(res.Scopes))
	}
	if len(res.Credentials) != 2 {
		t.Errorf("credentials = %d, want 2", len(res.Credentials))
	}
	if len(res.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(res.Targets))
	}

	byName := map[string]domain.Target{}
	for _, tg := range res.Targets {
		byName[tg.Name] = tg
	}

	prod := byName["prod-eks"]
	if prod.ID != "kubeconfig:context:prod-eks" {
		t.Errorf("prod ID = %q", prod.ID)
	}
	if prod.ScopeID != "kubeconfig:cluster:prod-eks-cluster" {
		t.Errorf("prod ScopeID = %q", prod.ScopeID)
	}
	if prod.CredentialID != "kubeconfig:user:prod-eks-user" {
		t.Errorf("prod CredentialID = %q", prod.CredentialID)
	}
	if prod.Health != domain.HealthUnknown { // exec-based, externally managed
		t.Errorf("prod Health = %q, want unknown", prod.Health)
	}
	if prod.Endpoint == "" {
		t.Error("prod Endpoint should carry the cluster server")
	}
	if prod.Metadata["current"] != "true" {
		t.Errorf("prod should be the current context, got current=%q", prod.Metadata["current"])
	}
	if prod.SystemLabels[domain.LabelPlatform] != "kubeconfig" {
		t.Errorf("prod platform label = %q", prod.SystemLabels[domain.LabelPlatform])
	}

	home := byName["homelab"]
	if home.Health != domain.HealthStatic { // client cert
		t.Errorf("homelab Health = %q, want static", home.Health)
	}
	if home.Metadata["current"] != "false" {
		t.Errorf("homelab should not be current, got current=%q", home.Metadata["current"])
	}
}

func TestDiscover_EmptyKubeconfigIsNotAnError(t *testing.T) {
	runner := execx.NewFakeRunner()
	runner.Responses["kubectl config view --raw -o json"] = execx.FakeResponse{Stdout: []byte(`{"apiVersion":"v1","kind":"Config"}`)}
	p := New(fakeResolver{path: "kubectl"}, runner)

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("empty kubeconfig should not error: %v", err)
	}
	if len(res.Targets) != 0 || len(res.Sources) != 0 {
		t.Errorf("empty kubeconfig should yield nothing, got %d targets / %d sources", len(res.Targets), len(res.Sources))
	}
}

func TestActivate_RunsUseContext(t *testing.T) {
	runner := execx.NewFakeRunner()
	runner.Responses["kubectl config use-context prod-eks"] = execx.FakeResponse{Stdout: []byte("Switched to context \"prod-eks\".")}
	p := New(fakeResolver{path: "kubectl"}, runner)

	err := p.Activate(context.Background(), domain.Target{Name: "prod-eks"})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if len(runner.Calls) != 1 || runner.Calls[0] != "kubectl config use-context prod-eks" {
		t.Errorf("expected use-context call, got %v", runner.Calls)
	}
}

func TestCapabilitiesAndRenew(t *testing.T) {
	p := New(fakeResolver{path: "kubectl"}, execx.NewFakeRunner())
	caps := p.Capabilities()
	if caps.CanRenew {
		t.Error("kubeconfig must not report CanRenew")
	}
	if !caps.CanSwitchContext || !caps.StaticCredentials || !caps.CanDiscoverScopes {
		t.Errorf("unexpected capabilities: %+v", caps)
	}
	// kubeconfig is an overlay: its contexts may duplicate clusters a cloud
	// provider owns natively, so it defers to them during cross-provider dedup.
	if !caps.OverlayProvider {
		t.Error("kubeconfig must report OverlayProvider")
	}
	if err := p.Renew(context.Background(), domain.Credential{}); !errors.Is(err, providers.ErrUnsupported) {
		t.Errorf("Renew should return ErrUnsupported, got %v", err)
	}
}
