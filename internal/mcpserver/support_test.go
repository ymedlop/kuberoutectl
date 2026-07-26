package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache/jsonstore"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// fakeProvider is a minimal Provider (+ ContextActivator) for tests: it returns
// a fixed discovery result and records the last activated target.
type fakeProvider struct {
	id        domain.ProviderID
	caps      domain.Capabilities
	result    providers.DiscoveryResult
	activated *domain.Target
}

func (f *fakeProvider) ID() domain.ProviderID             { return f.id }
func (f *fakeProvider) Capabilities() domain.Capabilities { return f.caps }
func (f *fakeProvider) Renew(context.Context, domain.Credential) error {
	return nil
}
func (f *fakeProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return f.result, nil
}
func (f *fakeProvider) Activate(_ context.Context, t domain.Target) error {
	f.activated = &t
	return nil
}

// newTestHandler builds a handler backed by a temp-dir JSON store seeded with
// snap and a registry containing provs.
func newTestHandler(t *testing.T, snap domain.InventorySnapshot, provs ...providers.Provider) *handler {
	t.Helper()
	dir := t.TempDir()
	store := jsonstore.New(dir, dir)
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	reg := providers.NewRegistry()
	for _, p := range provs {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}
	now := func() time.Time { return time.Unix(0, 0).UTC() }
	return &handler{d: Deps{
		Version:     "test",
		Registry:    reg,
		Discovery:   services.NewDiscoveryService(reg, store, now),
		Sources:     services.NewSourceService(store),
		Scopes:      services.NewScopeService(store),
		Credentials: services.NewCredentialService(store, reg),
		Targets:     services.NewTargetService(store),
		Selection:   services.NewSelectionService(store, reg, now),
		Collections: services.NewCollectionService(store, services.NewSelectorEngine()),
	}}
}

// seedTargets is a small two-provider inventory used across tests.
func seedTargets() domain.InventorySnapshot {
	return domain.InventorySnapshot{
		Targets: []domain.Target{
			{ID: "aws:eks:prod", Name: "eks-prod", ProviderID: "aws", Platform: "eks", Region: "eu-central-1", Health: domain.HealthValid, UserLabels: map[string]string{"env": "prod"}},
			{ID: "azure:aks:lab", Name: "aks-lab", ProviderID: "azure", Platform: "aks", Region: "westeurope", Health: domain.HealthValid, UserLabels: map[string]string{"env": "lab"}},
		},
		Credentials: []domain.Credential{
			{ID: "aws:default", ProviderID: "aws", Name: "default", Health: domain.HealthExpired, ActionHint: domain.ActionRenew, Metadata: map[string]string{"profile": "default", "auth_type": "sso"}},
		},
	}
}
