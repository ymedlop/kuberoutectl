package gcp

import (
	"context"
	"errors"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// errExit stands in for a non-zero CLI exit in fake responses.
var errExit = errors.New("exit status 1")

type fakeResolver struct{ path string }

func (f fakeResolver) Resolve(string) (string, error) { return f.path, nil }

// fullRunner wires the fixtures for a healthy, two-project discovery.
func fullRunner(t *testing.T) *execx.FakeRunner {
	t.Helper()
	r := execx.NewFakeRunner()
	r.Responses["gcloud config list --format=json"] = execx.FakeResponse{Stdout: readFixture(t, "config-list.json")}
	r.Responses["gcloud auth list --format=json"] = execx.FakeResponse{Stdout: readFixture(t, "auth-list.json")}
	r.Responses["gcloud projects list --format=json"] = execx.FakeResponse{Stdout: readFixture(t, "projects-list.json")}
	r.Responses["gcloud container clusters list --project platform-prod-123 --format=json"] = execx.FakeResponse{Stdout: readFixture(t, "clusters-list-prod.json")}
	r.Responses["gcloud container clusters list --project platform-lab-456 --format=json"] = execx.FakeResponse{Stdout: readFixture(t, "clusters-list-lab.json")}
	return r
}

func TestDiscover(t *testing.T) {
	p := New(fakeResolver{path: "gcloud"}, fullRunner(t))
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Sources) != 1 || len(res.Credentials) != 1 {
		t.Fatalf("sources=%d credentials=%d, want 1/1", len(res.Sources), len(res.Credentials))
	}
	if res.Credentials[0].Health != domain.HealthValid {
		t.Errorf("credential health = %q, want valid", res.Credentials[0].Health)
	}
	if len(res.Scopes) != 2 {
		t.Fatalf("scopes = %d, want 2", len(res.Scopes))
	}
	if len(res.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(res.Targets))
	}

	byName := map[string]domain.Target{}
	for _, tg := range res.Targets {
		byName[tg.Name] = tg
	}
	prod := byName["gke-prod-euw1"]
	if prod.ID != "gcp:platform-prod-123:europe-west1:gke-prod-euw1" {
		t.Errorf("prod ID = %q", prod.ID)
	}
	if prod.ScopeID != "gcp:project:platform-prod-123" {
		t.Errorf("prod ScopeID = %q", prod.ScopeID)
	}
	if prod.Region != "europe-west1" || prod.Platform != "gke" {
		t.Errorf("prod region/platform = %q/%q", prod.Region, prod.Platform)
	}
	if prod.Endpoint != "https://34.10.20.30" {
		t.Errorf("prod endpoint = %q", prod.Endpoint)
	}
	if prod.Metadata["kubernetes_version"] == "" {
		t.Error("prod missing kubernetes_version")
	}
}

func TestDiscover_LoggedOut(t *testing.T) {
	r := execx.NewFakeRunner()
	r.Responses["gcloud config list --format=json"] = execx.FakeResponse{Stdout: []byte(`{"core":{}}`)}
	r.Responses["gcloud auth list --format=json"] = execx.FakeResponse{Stdout: []byte(`[]`)}
	p := New(fakeResolver{path: "gcloud"}, r)

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("logged-out discovery should not error: %v", err)
	}
	if len(res.Credentials) != 1 {
		t.Fatalf("want a single credential surfacing the logged-out state, got %d", len(res.Credentials))
	}
	if res.Credentials[0].Health != domain.HealthExpired || res.Credentials[0].ActionHint != domain.ActionRenew {
		t.Errorf("logged-out credential = %s/%s, want expired/renew", res.Credentials[0].Health, res.Credentials[0].ActionHint)
	}
	if len(res.Scopes) != 0 || len(res.Targets) != 0 {
		t.Errorf("logged out should yield no scopes/targets, got %d/%d", len(res.Scopes), len(res.Targets))
	}
}

func TestDiscover_ProjectWithoutGKEIsSkipped(t *testing.T) {
	r := fullRunner(t)
	// Simulate the GKE API disabled for the lab project.
	r.Responses["gcloud container clusters list --project platform-lab-456 --format=json"] = execx.FakeResponse{Err: errExit}
	p := New(fakeResolver{path: "gcloud"}, r)

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("a failing project must not fail the whole sync: %v", err)
	}
	if len(res.Scopes) != 2 {
		t.Errorf("both projects are still scopes, got %d", len(res.Scopes))
	}
	if len(res.Targets) != 1 {
		t.Errorf("only the working project contributes targets, got %d", len(res.Targets))
	}
}

func TestActivate_RunsGetCredentials(t *testing.T) {
	r := execx.NewFakeRunner()
	r.Responses["gcloud container clusters get-credentials gke-prod-euw1 --location europe-west1 --project platform-prod-123"] = execx.FakeResponse{Stdout: []byte("Fetching cluster endpoint and auth data.")}
	p := New(fakeResolver{path: "gcloud"}, r)

	target := domain.Target{
		Name:     "gke-prod-euw1",
		Region:   "europe-west1",
		Metadata: map[string]string{"location": "europe-west1", "project": "platform-prod-123"},
	}
	if err := p.Activate(context.Background(), target); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected one get-credentials call, got %v", r.Calls)
	}
}

func TestActivate_MissingLocationErrors(t *testing.T) {
	p := New(fakeResolver{path: "gcloud"}, execx.NewFakeRunner())
	// No Region and no metadata location: get-credentials cannot be constructed.
	target := domain.Target{Name: "gke-orphan", Metadata: map[string]string{"project": "p"}}
	if err := p.Activate(context.Background(), target); err == nil {
		t.Fatal("expected an error when the target has no location")
	}
}

func TestBuildScope_NameFallsBackToProjectID(t *testing.T) {
	s := buildScope(gcpProject{ProjectID: "platform-prod-123", Name: ""})
	if s.Name != "platform-prod-123" {
		t.Errorf("scope name = %q, want fallback to projectId", s.Name)
	}
}

func TestRenew_RunsAuthLogin(t *testing.T) {
	r := execx.NewFakeRunner()
	r.Responses["gcloud auth login yeray@example.com"] = execx.FakeResponse{Stdout: []byte("You are now logged in.")}
	p := New(fakeResolver{path: "gcloud"}, r)

	cred := domain.Credential{Metadata: map[string]string{"account": "yeray@example.com"}}
	if err := p.Renew(context.Background(), cred); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if len(r.Calls) != 1 || r.Calls[0] != "gcloud auth login yeray@example.com" {
		t.Errorf("expected auth login call, got %v", r.Calls)
	}
}

func TestCapabilities(t *testing.T) {
	caps := New(fakeResolver{path: "gcloud"}, execx.NewFakeRunner()).Capabilities()
	if !caps.CanRenew || !caps.CanSwitchContext || !caps.CanDiscoverScopes {
		t.Errorf("unexpected capabilities: %+v", caps)
	}
	if caps.StaticCredentials {
		t.Error("GCP OAuth login is renewable; StaticCredentials should be false")
	}
}
