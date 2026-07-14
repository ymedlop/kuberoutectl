package gcp

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(readFixture(t, "config-list.json"))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Core.Account != "yeray@example.com" {
		t.Errorf("account = %q", cfg.Core.Account)
	}
	if cfg.Core.Project != "platform-prod-123" {
		t.Errorf("project = %q", cfg.Core.Project)
	}
}

func TestParseAuthList(t *testing.T) {
	accounts, err := parseAuthList(readFixture(t, "auth-list.json"))
	if err != nil {
		t.Fatalf("parseAuthList: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(accounts))
	}
	if accounts[0].Account != "yeray@example.com" || accounts[0].Status != "ACTIVE" {
		t.Errorf("unexpected first account: %+v", accounts[0])
	}
}

func TestParseProjects(t *testing.T) {
	projects, err := parseProjects(readFixture(t, "projects-list.json"))
	if err != nil {
		t.Fatalf("parseProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	if projects[0].ProjectID != "platform-prod-123" || projects[0].Name != "Platform Prod" {
		t.Errorf("unexpected project: %+v", projects[0])
	}
}

func TestParseClusters(t *testing.T) {
	clusters, err := parseClusters(readFixture(t, "clusters-list-prod.json"))
	if err != nil {
		t.Fatalf("parseClusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("clusters = %d, want 1", len(clusters))
	}
	c := clusters[0]
	if c.Name != "gke-prod-euw1" || c.Location != "europe-west1" || c.Status != "RUNNING" {
		t.Errorf("unexpected cluster: %+v", c)
	}
	if c.CurrentMasterVersion == "" || c.Endpoint == "" {
		t.Errorf("cluster missing version/endpoint: %+v", c)
	}
}
