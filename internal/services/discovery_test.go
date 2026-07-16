package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// recordProgress captures Step calls so tests can assert user-facing messages.
type recordProgress struct{ steps []string }

func (r *recordProgress) Step(format string, args ...any) {
	r.steps = append(r.steps, fmt.Sprintf(format, args...))
}

func (r *recordProgress) contains(sub string) bool {
	for _, s := range r.steps {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// memStore is an in-memory CacheStore for service tests.
type memStore struct {
	snap        domain.InventorySnapshot
	userLabels  map[domain.TargetID]map[string]string
	collections []domain.Collection
	selection   domain.Selection
	hidden      []domain.TargetID
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
func (m *memStore) LoadHiddenTargets() ([]domain.TargetID, error) { return m.hidden, nil }
func (m *memStore) SaveHiddenTargets(ids []domain.TargetID) error { m.hidden = ids; return nil }

// fakeProvider returns a fixed discovery result. caps is configurable so tests
// can exercise capability-gated behavior (e.g. OverlayProvider dedup).
type fakeProvider struct {
	id   domain.ProviderID
	res  providers.DiscoveryResult
	caps domain.Capabilities
}

func (f fakeProvider) ID() domain.ProviderID             { return f.id }
func (f fakeProvider) Capabilities() domain.Capabilities { return f.caps }
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

// The dedup guarantee: an overlay provider's target (kubeconfig context) is
// suppressed when a native provider already owns that endpoint — no matter which
// provider was synced first.
const dupEndpoint = "https://ABC123.gr7.eu-central-1.eks.amazonaws.com"

func TestSync_KubeconfigAfterAWS_SuppressesDuplicate(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Targets: []domain.Target{
			{ID: "aws:eks", ProviderID: "aws", Name: "eks-prod", Endpoint: dupEndpoint},
		},
	}

	reg := providers.NewRegistry()
	_ = reg.Register(fakeProvider{
		id:   "kubeconfig",
		caps: domain.Capabilities{OverlayProvider: true},
		res: providers.DiscoveryResult{Targets: []domain.Target{
			{ID: "kubeconfig:context:eks", ProviderID: "kubeconfig", Name: "prod-eks", Endpoint: dupEndpoint},
			{ID: "kubeconfig:context:homelab", ProviderID: "kubeconfig", Name: "homelab", Endpoint: "https://192.168.1.10:6443"},
		}},
	})

	prog := &recordProgress{}
	snap, err := NewDiscoveryService(reg, store, fixedNow).Sync(context.Background(), "kubeconfig", prog)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	ids := targetIDs(snap.Targets)
	if contains(ids, "kubeconfig:context:eks") {
		t.Errorf("kubeconfig duplicate of a native cluster not suppressed: %v", ids)
	}
	if !contains(ids, "aws:eks") {
		t.Errorf("native aws target dropped: %v", ids)
	}
	if !contains(ids, "kubeconfig:context:homelab") {
		t.Errorf("non-duplicate kubeconfig context wrongly suppressed: %v", ids)
	}
	// The suppression must be observable to the operator (documented CLI output).
	if !prog.contains("suppressed 1 overlay context(s) already discovered natively") {
		t.Errorf("expected a suppression progress message, got %v", prog.steps)
	}
}

func TestSync_AWSAfterKubeconfig_SuppressesDuplicate(t *testing.T) {
	// Reverse order: the kubeconfig target already sits in the cache; syncing the
	// native provider must still make native win.
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Targets: []domain.Target{
			{ID: "kubeconfig:context:eks", ProviderID: "kubeconfig", Name: "prod-eks", Endpoint: dupEndpoint},
		},
	}

	reg := providers.NewRegistry()
	// Both providers must be registered so isOverlay can classify the cached
	// kubeconfig target during an aws sync.
	_ = reg.Register(fakeProvider{id: "kubeconfig", caps: domain.Capabilities{OverlayProvider: true}})
	_ = reg.Register(fakeProvider{
		id:   "aws",
		caps: domain.Capabilities{CanRenew: true},
		res: providers.DiscoveryResult{Targets: []domain.Target{
			{ID: "aws:eks", ProviderID: "aws", Name: "eks-prod", Endpoint: dupEndpoint},
		}},
	})

	snap, err := NewDiscoveryService(reg, store, fixedNow).Sync(context.Background(), "aws", nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	ids := targetIDs(snap.Targets)
	if contains(ids, "kubeconfig:context:eks") {
		t.Errorf("cached kubeconfig duplicate not suppressed after native sync: %v", ids)
	}
	if !contains(ids, "aws:eks") {
		t.Errorf("native aws target missing: %v", ids)
	}
}

func TestSync_UnregisteredProviderTreatedAsNative(t *testing.T) {
	// A cached target for a provider no longer registered must not panic on the
	// registry lookup and must be treated as non-overlay (never suppressed); it
	// can still own an endpoint that suppresses an overlay duplicate.
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Targets: []domain.Target{
			{ID: "ghost:cluster", ProviderID: "ghost", Name: "legacy", Endpoint: dupEndpoint},
		},
	}

	reg := providers.NewRegistry()
	_ = reg.Register(fakeProvider{
		id:   "kubeconfig",
		caps: domain.Capabilities{OverlayProvider: true},
		res: providers.DiscoveryResult{Targets: []domain.Target{
			{ID: "kubeconfig:context:eks", ProviderID: "kubeconfig", Name: "prod-eks", Endpoint: dupEndpoint},
		}},
	})

	snap, err := NewDiscoveryService(reg, store, fixedNow).Sync(context.Background(), "kubeconfig", nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	ids := targetIDs(snap.Targets)
	if !contains(ids, "ghost:cluster") {
		t.Errorf("unregistered-provider target wrongly dropped: %v", ids)
	}
	if contains(ids, "kubeconfig:context:eks") {
		t.Errorf("overlay duplicate not suppressed by unregistered native owner: %v", ids)
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
