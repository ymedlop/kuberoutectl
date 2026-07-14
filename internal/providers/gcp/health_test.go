package gcp

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestActiveAccount(t *testing.T) {
	accounts := []gcpAuthAccount{
		{Account: "old@example.com", Status: ""},
		{Account: "yeray@example.com", Status: "ACTIVE"},
	}
	// The ACTIVE entry wins regardless of config.
	got, ok := activeAccount("config@example.com", accounts)
	if !ok || got != "yeray@example.com" {
		t.Errorf("active from auth list: got %q ok=%v", got, ok)
	}

	// No ACTIVE entry: fall back to the config account.
	got, ok = activeAccount("config@example.com", []gcpAuthAccount{{Account: "x", Status: ""}})
	if !ok || got != "config@example.com" {
		t.Errorf("fallback to config: got %q ok=%v", got, ok)
	}

	// Nothing active and no config account: not authed.
	if _, ok := activeAccount("", nil); ok {
		t.Error("expected not authed when no active account and no config account")
	}
}

func TestMapGCPHealth(t *testing.T) {
	h, a := mapGCPHealth(true)
	if h != domain.HealthValid || a != domain.ActionUse {
		t.Errorf("authed: got %s/%s, want valid/use", h, a)
	}
	h, a = mapGCPHealth(false)
	if h != domain.HealthExpired || a != domain.ActionRenew {
		t.Errorf("not authed: got %s/%s, want expired/renew", h, a)
	}
}
