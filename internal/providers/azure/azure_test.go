package azure

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// fakeResolver always resolves BinaryName to a fixed path.
type fakeResolver struct{ path string }

func (f fakeResolver) Resolve(string) (string, error) { return f.path, nil }

// tokenEpoch is the epoch in testdata/access-token.json.
const tokenEpoch int64 = 1784982600

func TestParseAccounts(t *testing.T) {
	accounts, err := parseAccounts(readFixture(t, "account-list.json"))
	if err != nil {
		t.Fatalf("parseAccounts: %v", err)
	}
	if len(accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(accounts))
	}
	if accounts[0].Name != "Platform Production" || !accounts[0].IsDefault {
		t.Errorf("unexpected first account: %+v", accounts[0])
	}
	if accounts[2].State != "Disabled" {
		t.Errorf("expected third account disabled, got %q", accounts[2].State)
	}
}

func TestParseAccessToken_PrefersEpoch(t *testing.T) {
	tok, err := parseAccessToken(readFixture(t, "access-token.json"))
	if err != nil {
		t.Fatalf("parseAccessToken: %v", err)
	}
	want := time.Unix(tokenEpoch, 0).UTC()
	if !tok.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v (epoch preferred over local string)", tok.ExpiresAt, want)
	}
}

func TestMapHealth(t *testing.T) {
	exp := time.Unix(tokenEpoch, 0).UTC()
	tok := azToken{ExpiresAt: exp}
	cases := []struct {
		name   string
		now    time.Time
		health domain.AccessHealth
		action domain.ActionHint
	}{
		{"valid", exp.Add(-1 * time.Hour), domain.HealthValid, domain.ActionUse},
		{"expiring", exp.Add(-2 * time.Minute), domain.HealthExpiring, domain.ActionRenew},
		{"expired", exp.Add(1 * time.Minute), domain.HealthExpired, domain.ActionRenew},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, a := mapHealth(tok, c.now, defaultExpiringWithin)
			if h != c.health || a != c.action {
				t.Errorf("got (%s,%s), want (%s,%s)", h, a, c.health, c.action)
			}
		})
	}
}

// newFakeAzProvider wires a provider over a FakeRunner primed with the az
// command outputs, with a fixed clock that makes the token look valid.
func newFakeAzProvider(t *testing.T) *Provider {
	t.Helper()
	runner := execx.NewFakeRunner()
	runner.Responses["az account list --output json"] = execx.FakeResponse{Stdout: readFixture(t, "account-list.json")}
	runner.Responses["az account get-access-token --output json"] = execx.FakeResponse{Stdout: readFixture(t, "access-token.json")}
	runner.Responses["az aks list --subscription aaaaaaaa-0000-0000-0000-000000000001 --output json"] = execx.FakeResponse{Stdout: readFixture(t, "aks-list-prod.json")}
	runner.Responses["az aks list --subscription aaaaaaaa-0000-0000-0000-000000000002 --output json"] = execx.FakeResponse{Stdout: readFixture(t, "aks-list-lab.json")}

	p := New(fakeResolver{path: "az"}, runner)
	// Fix the clock an hour before token expiry -> token is valid.
	p.now = func() time.Time { return time.Unix(tokenEpoch, 0).UTC().Add(-1 * time.Hour) }
	return p
}

func TestDiscover_FullInventory(t *testing.T) {
	p := newFakeAzProvider(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(res.Sources) != 1 || res.Sources[0].ID != sourceID {
		t.Fatalf("expected single azure-cli source, got %+v", res.Sources)
	}

	// Two enabled subscriptions -> two scopes; disabled one is still a scope
	// (it exists) but yields no targets.
	if len(res.Scopes) != 3 {
		t.Fatalf("expected 3 scopes, got %d", len(res.Scopes))
	}

	// Same user across all subscriptions -> a single deduped credential.
	if len(res.Credentials) != 1 {
		t.Fatalf("expected 1 deduped credential, got %d: %+v", len(res.Credentials), res.Credentials)
	}
	if res.Credentials[0].Health != domain.HealthValid {
		t.Errorf("credential health = %s, want valid", res.Credentials[0].Health)
	}

	// 2 prod + 1 lab clusters; disabled subscription skipped.
	if len(res.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(res.Targets))
	}
	// Deterministic sort by ID: prod-neu, prod-weu, lab-weu (by resource id).
	var names []string
	for _, tg := range res.Targets {
		names = append(names, tg.Name)
		if tg.Platform != "aks" {
			t.Errorf("target %s platform = %q, want aks", tg.Name, tg.Platform)
		}
		if tg.SystemLabels[domain.LabelProvider] != "azure" {
			t.Errorf("target %s missing provider system label", tg.Name)
		}
		if tg.SystemLabels[domain.LabelHealth] != string(domain.HealthValid) {
			t.Errorf("target %s health label = %q", tg.Name, tg.SystemLabels[domain.LabelHealth])
		}
		if len(tg.UserLabels) != 0 {
			t.Errorf("provider must not set user labels, got %+v", tg.UserLabels)
		}
	}

	// A prod target must reference the prod subscription scope.
	for _, tg := range res.Targets {
		if tg.Name == "aks-prod-weu" {
			if tg.ScopeID != "aaaaaaaa-0000-0000-0000-000000000001" {
				t.Errorf("aks-prod-weu scope = %q, want prod subscription", tg.ScopeID)
			}
			if tg.Endpoint != "https://aks-prod-weu-dns-abc123.hcp.westeurope.azmk8s.io" {
				t.Errorf("aks-prod-weu endpoint = %q", tg.Endpoint)
			}
			if tg.KubernetesVersion != "1.28.3" {
				t.Errorf("aks-prod-weu KubernetesVersion = %q, want 1.28.3", tg.KubernetesVersion)
			}
		}
	}
}

func TestDiscover_NotLoggedIn(t *testing.T) {
	runner := execx.NewFakeRunner()
	runner.Responses["az account list --output json"] = execx.FakeResponse{
		Stderr: []byte("Please run 'az login' to setup account."),
		Err:    &exitError{},
	}
	p := New(fakeResolver{path: "az"}, runner)

	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover should not error when logged out: %v", err)
	}
	if len(res.Credentials) != 1 || res.Credentials[0].Health != domain.HealthExpired {
		t.Fatalf("expected one expired credential, got %+v", res.Credentials)
	}
	if res.Credentials[0].ActionHint != domain.ActionRenew {
		t.Errorf("logged-out credential action = %s, want renew", res.Credentials[0].ActionHint)
	}
	if len(res.Targets) != 0 {
		t.Errorf("expected no targets when logged out, got %d", len(res.Targets))
	}
}

// exitError is a stand-in for a non-zero command exit.
type exitError struct{}

func (*exitError) Error() string { return "exit status 1" }
