package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// activatableProvider implements Provider + ContextActivator and records
// whether Activate was called.
type activatableProvider struct {
	id          domain.ProviderID
	canSwitch   bool
	activateErr error
	activated   *domain.Target
}

func (a *activatableProvider) ID() domain.ProviderID { return a.id }
func (a *activatableProvider) Capabilities() domain.Capabilities {
	return domain.Capabilities{CanSwitchContext: a.canSwitch}
}
func (a *activatableProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return providers.DiscoveryResult{}, nil
}
func (a *activatableProvider) Renew(context.Context, domain.Credential) error { return nil }
func (a *activatableProvider) Activate(_ context.Context, t domain.Target) error {
	if a.activateErr != nil {
		return a.activateErr
	}
	tt := t
	a.activated = &tt
	return nil
}

func storeWithSelTarget() *memStore {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{Targets: []domain.Target{
		{ID: "t1", ProviderID: "azure", Name: "aks-prod"},
	}}
	return m
}

func TestUseTarget_ActivatesByDefault(t *testing.T) {
	store := storeWithSelTarget()
	prov := &activatableProvider{id: "azure", canSwitch: true}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)

	svc := NewSelectionService(store, reg, fixedNow)
	if _, err := svc.UseTarget(context.Background(), "t1", true); err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if prov.activated == nil || prov.activated.ID != "t1" {
		t.Errorf("expected Activate to be called for t1, got %+v", prov.activated)
	}
	if store.selection.TargetID != "t1" {
		t.Errorf("selection not recorded: %+v", store.selection)
	}
}

func TestUseTarget_NoKubeconfigSkipsActivation(t *testing.T) {
	store := storeWithSelTarget()
	prov := &activatableProvider{id: "azure", canSwitch: true}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)

	svc := NewSelectionService(store, reg, fixedNow)
	if _, err := svc.UseTarget(context.Background(), "t1", false); err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if prov.activated != nil {
		t.Errorf("Activate must not be called with activate=false")
	}
	if store.selection.TargetID != "t1" {
		t.Errorf("selection should still be recorded: %+v", store.selection)
	}
}

func TestStatus_ResolvesSelectedTarget(t *testing.T) {
	store := storeWithSelTarget()
	svc := NewSelectionService(store, nil, fixedNow)

	// Nothing selected yet: empty status, no error.
	st, err := svc.Status()
	if err != nil {
		t.Fatalf("Status (empty): %v", err)
	}
	if st.Target != nil || st.Collection != nil {
		t.Errorf("empty selection should resolve nothing: %+v", st)
	}

	store.selection = domain.Selection{TargetID: "t1", UpdatedAt: fixedNow()}
	st, err = svc.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Target == nil || st.Target.ID != "t1" {
		t.Fatalf("expected target t1 resolved, got %+v", st.Target)
	}
	if st.Target.Alias == "" {
		t.Error("resolved target should carry an alias")
	}
	if st.SyncedAt != store.snap.SyncedAt {
		t.Errorf("SyncedAt not propagated: %v", st.SyncedAt)
	}
}

func TestStatus_SelectedTargetGoneFromCache(t *testing.T) {
	store := storeWithSelTarget()
	store.selection = domain.Selection{TargetID: "vanished", UpdatedAt: fixedNow()}
	svc := NewSelectionService(store, nil, fixedNow)

	// A selection pointing at a target that no longer exists (removed by a
	// resync) is stale, not an error: the selection is reported, Target is nil.
	st, err := svc.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Selection.TargetID != "vanished" || st.Target != nil {
		t.Errorf("stale selection should surface with nil target: %+v", st)
	}
}

func TestUseTarget_ActivateUnsupportedErrors(t *testing.T) {
	store := storeWithSelTarget()
	prov := &activatableProvider{id: "azure", canSwitch: false} // cannot switch context
	reg := providers.NewRegistry()
	_ = reg.Register(prov)

	svc := NewSelectionService(store, reg, fixedNow)
	if _, err := svc.UseTarget(context.Background(), "t1", true); err == nil {
		t.Fatal("expected error activating a provider that cannot switch context")
	}
	// Selection must NOT be recorded when a requested activation fails.
	if store.selection.TargetID != "" {
		t.Errorf("selection should not be recorded on activation failure: %+v", store.selection)
	}
}

func TestUseTarget_ActivationFailureDoesNotRecord(t *testing.T) {
	store := storeWithSelTarget()
	prov := &activatableProvider{id: "azure", canSwitch: true, activateErr: errors.New("az login required")}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)

	svc := NewSelectionService(store, reg, fixedNow)
	if _, err := svc.UseTarget(context.Background(), "t1", true); err == nil {
		t.Fatal("expected activation error to propagate")
	}
	if store.selection.TargetID != "" {
		t.Errorf("selection should not be recorded when activation fails: %+v", store.selection)
	}
}
