package services

import (
	"context"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// memStore is an in-memory CacheStore for service tests.
type memStore struct {
	snap        domain.InventorySnapshot
	userLabels  map[domain.TargetID]map[string]string
	collections []domain.Collection
	selection   domain.Selection
}

func newMemStore() *memStore {
	return &memStore{userLabels: map[domain.TargetID]map[string]string{}}
}

func (m *memStore) LoadSnapshot() (domain.InventorySnapshot, error) { return m.snap, nil }
func (m *memStore) SaveSnapshot(s domain.InventorySnapshot) error   { m.snap = s; return nil }
func (m *memStore) LoadUserLabels() (map[domain.TargetID]map[string]string, error) {
	return m.userLabels, nil
}
func (m *memStore) SaveUserLabels(l map[domain.TargetID]map[string]string) error {
	m.userLabels = l
	return nil
}
func (m *memStore) LoadCollections() ([]domain.Collection, error) { return m.collections, nil }
func (m *memStore) SaveCollections(c []domain.Collection) error   { m.collections = c; return nil }
func (m *memStore) LoadSelection() (domain.Selection, error)      { return m.selection, nil }
func (m *memStore) SaveSelection(s domain.Selection) error        { m.selection = s; return nil }

// fakeProvider returns a fixed discovery result.
type fakeProvider struct {
	id  domain.ProviderID
	res providers.DiscoveryResult
}

func (f fakeProvider) ID() domain.ProviderID             { return f.id }
func (f fakeProvider) Capabilities() domain.Capabilities { return domain.Capabilities{CanRenew: true} }
func (f fakeProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return f.res, nil
}
func (f fakeProvider) Renew(context.Context, domain.Credential) error { return nil }

func fixedNow() time.Time { return time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC) }

// This is the headline guarantee: a resync rediscovers a target (with empty
// UserLabels from the provider) but the user's labels, stored separately, are
// re-attached and survive.
func TestSync_PreservesUserLabelsAcrossResync(t *testing.T) {
	store := newMemStore()
	store.userLabels["t1"] = map[string]string{"env": "prod", "team": "platform"}

	reg := providers.NewRegistry()
	_ = reg.Register(fakeProvider{
		id: "azure",
		res: providers.DiscoveryResult{
			Targets: []domain.Target{{
				ID:           "t1",
				ProviderID:   "azure",
				Name:         "aks-prod",
				SystemLabels: map[string]string{domain.LabelProvider: "azure"},
				// UserLabels intentionally empty, as a real provider returns.
			}},
		},
	})

	svc := NewDiscoveryService(reg, store, fixedNow)
	snap, err := svc.Sync(context.Background(), "azure", nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(snap.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(snap.Targets))
	}
	ul := snap.Targets[0].UserLabels
	if ul["env"] != "prod" || ul["team"] != "platform" {
		t.Fatalf("user labels not re-attached after resync: %+v", ul)
	}
	// System labels remain provider-owned.
	if snap.Targets[0].SystemLabels[domain.LabelProvider] != "azure" {
		t.Errorf("system label lost")
	}
}

// Syncing one provider must not drop another provider's inventory.
func TestSync_ReplacesOnlyOwnProvider(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Targets: []domain.Target{
			{ID: "aws-1", ProviderID: "aws", Name: "eks-old"},
			{ID: "az-old", ProviderID: "azure", Name: "aks-stale"},
		},
		Credentials: []domain.Credential{{ID: "aws-c", ProviderID: "aws"}},
	}

	reg := providers.NewRegistry()
	_ = reg.Register(fakeProvider{
		id: "azure",
		res: providers.DiscoveryResult{
			Targets:     []domain.Target{{ID: "az-new", ProviderID: "azure", Name: "aks-fresh"}},
			Credentials: []domain.Credential{{ID: "az-c", ProviderID: "azure"}},
		},
	})

	svc := NewDiscoveryService(reg, store, fixedNow)
	snap, err := svc.Sync(context.Background(), "azure", nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	var names []string
	for _, tg := range snap.Targets {
		names = append(names, tg.Name)
	}
	// aws target preserved; stale azure target replaced by the fresh one.
	if !contains(names, "eks-old") {
		t.Errorf("aws target dropped by azure sync: %v", names)
	}
	if contains(names, "aks-stale") {
		t.Errorf("stale azure target not replaced: %v", names)
	}
	if !contains(names, "aks-fresh") {
		t.Errorf("fresh azure target missing: %v", names)
	}

	// Credentials follow the same rule.
	var credProviders []domain.ProviderID
	for _, c := range snap.Credentials {
		credProviders = append(credProviders, c.ProviderID)
	}
	if len(snap.Credentials) != 2 {
		t.Errorf("expected aws + azure credentials, got %+v", credProviders)
	}
}

func TestSync_UnknownProvider(t *testing.T) {
	svc := NewDiscoveryService(providers.NewRegistry(), newMemStore(), fixedNow)
	if _, err := svc.Sync(context.Background(), "nope", nil); err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
