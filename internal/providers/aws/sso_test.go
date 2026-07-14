package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/execx"
)

func TestParseSSOSession(t *testing.T) {
	cfg := `
[sso-session mycompany]
sso_start_url = https://mycompany.awsapps.com/start
sso_region = eu-west-1
sso_registration_scopes = sso:account:access

[profile existing]
region = us-east-1
`
	s, err := parseSSOSession(cfg, "mycompany")
	if err != nil {
		t.Fatalf("parseSSOSession: %v", err)
	}
	if s.StartURL != "https://mycompany.awsapps.com/start" || s.Region != "eu-west-1" {
		t.Fatalf("unexpected session: %+v", s)
	}
	if _, err := parseSSOSession(cfg, "nope"); err == nil {
		t.Error("expected error for missing session")
	}
}

func TestFindSSOToken(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	start := "https://mycompany.awsapps.com/start"

	// A stale registration file (no token) — must be ignored.
	os.WriteFile(filepath.Join(dir, "reg.json"), []byte(`{"clientId":"x","clientSecret":"y"}`), 0o600)
	// An expired token — must be ignored.
	os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"startUrl":"`+start+`","accessToken":"OLD","expiresAt":"2020-01-01T00:00:00Z"}`), 0o600)
	// A valid token.
	os.WriteFile(filepath.Join(dir, "good.json"), []byte(`{"startUrl":"`+start+`","accessToken":"GOOD","region":"eu-west-1","expiresAt":"2026-07-14T18:00:00Z"}`), 0o600)

	tok, err := findSSOToken(dir, start, now)
	if err != nil {
		t.Fatalf("findSSOToken: %v", err)
	}
	if tok.AccessToken != "GOOD" {
		t.Errorf("expected GOOD token, got %q", tok.AccessToken)
	}

	// Missing cache dir -> login required.
	if _, err := findSSOToken(filepath.Join(dir, "nope"), start, now); !errors.Is(err, ErrSSOLoginRequired) {
		t.Errorf("expected ErrSSOLoginRequired for missing cache, got %v", err)
	}
	// Only an expired token present -> login required.
	os.Remove(filepath.Join(dir, "good.json"))
	if _, err := findSSOToken(dir, start, now); !errors.Is(err, ErrSSOLoginRequired) {
		t.Errorf("expected ErrSSOLoginRequired when only expired token, got %v", err)
	}
}

func TestPickRole(t *testing.T) {
	roles := []string{"ReadOnly", "AdministratorAccess", "PowerUser"}
	if r, _ := pickRole(roles, ""); r != "AdministratorAccess" {
		t.Errorf("default should prefer AdministratorAccess, got %q", r)
	}
	if r, _ := pickRole(roles, "PowerUser"); r != "PowerUser" {
		t.Errorf("explicit preference should win, got %q", r)
	}
	if r, _ := pickRole([]string{"ReadOnly", "Billing"}, ""); r != "Billing" {
		t.Errorf("no admin -> first alphabetically (Billing), got %q", r)
	}
	if _, ok := pickRole(nil, ""); ok {
		t.Error("empty roles should report ok=false")
	}
}

func TestProfileNameSanitizes(t *testing.T) {
	got := profileName(ssoAccount{AccountName: "Platform Prod (EU)", AccountID: "111"}, "AdministratorAccess")
	if got != "kr-Platform-Prod-EU-AdministratorAccess" {
		t.Errorf("unexpected profile name: %q", got)
	}
	// Falls back to account id when name is empty.
	if got := profileName(ssoAccount{AccountID: "222"}, "Role"); got != "kr-222-Role" {
		t.Errorf("unexpected fallback name: %q", got)
	}
}

func TestParseSSOAccountsAndRoles(t *testing.T) {
	accs, err := parseSSOAccounts([]byte(`{"accountList":[{"accountId":"111","accountName":"Prod"},{"accountId":"222","accountName":"Dev"}]}`))
	if err != nil || len(accs) != 2 || accs[0].AccountName != "Prod" {
		t.Fatalf("parseSSOAccounts: %v %+v", err, accs)
	}
	roles, err := parseSSORoles([]byte(`{"roleList":[{"roleName":"AdministratorAccess"},{"roleName":"ReadOnly"}]}`))
	if err != nil || len(roles) != 2 {
		t.Fatalf("parseSSORoles: %v %+v", err, roles)
	}
}

// End-to-end population against a fake aws + a fake config/cache on disk.
func TestPopulateSSOProfiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cacheDir := filepath.Join(dir, "cache")
	os.MkdirAll(cacheDir, 0o700)

	start := "https://mycompany.awsapps.com/start"
	os.WriteFile(configPath, []byte("[sso-session mycompany]\nsso_start_url = "+start+"\nsso_region = eu-west-1\n\n[profile kr-Prod-AdministratorAccess]\nregion = eu-west-1\n"), 0o600)
	os.WriteFile(filepath.Join(cacheDir, "t.json"), []byte(`{"startUrl":"`+start+`","accessToken":"TOK","region":"eu-west-1","expiresAt":"2999-01-01T00:00:00Z"}`), 0o600)

	runner := execx.NewFakeRunner()
	runner.Responses["aws sso list-accounts --access-token TOK --region eu-west-1 --output json"] =
		execx.FakeResponse{Stdout: []byte(`{"accountList":[{"accountId":"111","accountName":"Prod"},{"accountId":"222","accountName":"Dev"}]}`)}
	runner.Responses["aws sso list-account-roles --account-id 111 --access-token TOK --region eu-west-1 --output json"] =
		execx.FakeResponse{Stdout: []byte(`{"roleList":[{"roleName":"AdministratorAccess"}]}`)}
	runner.Responses["aws sso list-account-roles --account-id 222 --access-token TOK --region eu-west-1 --output json"] =
		execx.FakeResponse{Stdout: []byte(`{"roleList":[{"roleName":"AdministratorAccess"},{"roleName":"ReadOnly"}]}`)}

	p := New(fakeResolver{path: "aws"}, runner)

	res, err := p.PopulateSSOProfiles(context.Background(), SSOPopulateOptions{
		SessionName: "mycompany",
		ConfigPath:  configPath,
		CacheDir:    cacheDir,
	})
	if err != nil {
		t.Fatalf("PopulateSSOProfiles: %v", err)
	}
	if res.Accounts != 2 {
		t.Errorf("expected 2 accounts, got %d", res.Accounts)
	}
	// Prod's profile already existed -> skipped; Dev's is written.
	if len(res.Written) != 1 || res.Written[0] != "kr-Dev-AdministratorAccess" {
		t.Errorf("unexpected written: %+v", res.Written)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "kr-Prod-AdministratorAccess" {
		t.Errorf("unexpected skipped: %+v", res.Skipped)
	}

	final, _ := os.ReadFile(configPath)
	block := "[profile kr-Dev-AdministratorAccess]\nsso_session = mycompany\nsso_account_id = 222\nsso_role_name = AdministratorAccess\nregion = eu-west-1"
	if !strings.Contains(string(final), block) {
		t.Errorf("generated profile block missing or malformed:\n%s", final)
	}
}

func TestPopulateSSOProfiles_RequiresLogin(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	os.WriteFile(configPath, []byte("[sso-session s]\nsso_start_url = https://x.awsapps.com/start\nsso_region = eu-west-1\n"), 0o600)

	p := New(fakeResolver{path: "aws"}, execx.NewFakeRunner())
	_, err := p.PopulateSSOProfiles(context.Background(), SSOPopulateOptions{
		SessionName: "s", ConfigPath: configPath, CacheDir: filepath.Join(dir, "cache"),
	})
	if err == nil || !strings.Contains(err.Error(), "aws sso login") {
		t.Fatalf("expected a login-required error, got %v", err)
	}
}
