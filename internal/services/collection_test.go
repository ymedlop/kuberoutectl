package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func storeWithTargets(targets ...domain.Target) *memStore {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{Targets: targets}
	return m
}

func prodAndLab() []domain.Target {
	return []domain.Target{
		{ID: "prod-1", Name: "aks-prod-weu", Region: "westeurope",
			SystemLabels: map[string]string{domain.LabelRegion: "westeurope"},
			UserLabels:   map[string]string{"env": "prod"}},
		{ID: "prod-2", Name: "aks-prod-neu", Region: "northeurope",
			SystemLabels: map[string]string{domain.LabelRegion: "northeurope"},
			UserLabels:   map[string]string{"env": "prod"}},
		{ID: "lab-1", Name: "aks-lab-weu", Region: "westeurope",
			SystemLabels: map[string]string{domain.LabelRegion: "westeurope"},
			UserLabels:   map[string]string{"env": "lab"}},
	}
}

func TestCollection_CreateRequiresMembership(t *testing.T) {
	svc := NewCollectionService(newMemStore(), nil)
	if _, err := svc.Create("empty", "", domain.LabelSelector{}, nil); err == nil {
		t.Fatal("expected error creating a collection with no selector and no static IDs")
	}
}

func TestCollection_CreateRejectsDuplicate(t *testing.T) {
	svc := NewCollectionService(storeWithTargets(prodAndLab()...), nil)
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if _, err := svc.Create("production", "", sel, nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.Create("production", "", sel, nil); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestCollection_ResolveSelector(t *testing.T) {
	store := storeWithTargets(prodAndLab()...)
	svc := NewCollectionService(store, nil)
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if _, err := svc.Create("production", "", sel, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	members, err := svc.Resolve("production")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 prod members, got %d: %+v", len(members), members)
	}
	// Deterministic sort by ID.
	if members[0].ID != "prod-1" || members[1].ID != "prod-2" {
		t.Errorf("members not sorted by ID: %+v", members)
	}
}

// Newly discovered targets that match the selector should join automatically —
// the defining property of a saved view versus a static folder.
func TestCollection_SelectorPicksUpNewTargets(t *testing.T) {
	store := storeWithTargets(prodAndLab()...)
	svc := NewCollectionService(store, nil)
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if _, err := svc.Create("production", "", sel, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate a resync adding a new prod cluster.
	store.snap.Targets = append(store.snap.Targets, domain.Target{
		ID: "prod-3", Name: "aks-prod-us", UserLabels: map[string]string{"env": "prod"},
	})
	members, err := svc.Resolve("production")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected new target to join collection automatically, got %d", len(members))
	}
}

func TestCollection_StaticUnionAndDedup(t *testing.T) {
	store := storeWithTargets(prodAndLab()...)
	svc := NewCollectionService(store, nil)
	// Selector picks prod; static adds a lab target and (redundantly) a prod one.
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if _, err := svc.Create("mixed", "", sel, []domain.TargetID{"lab-1", "prod-1", "ghost"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	members, err := svc.Resolve("mixed")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// prod-1, prod-2 (selector) + lab-1 (static); prod-1 deduped; ghost skipped.
	if len(members) != 3 {
		t.Fatalf("expected 3 unique members, got %d: %+v", len(members), members)
	}
}

// The visible/hidden selector keys must be honest in collection resolution too
// (the selector engine is shared with target list), so a collection selecting
// hidden targets resolves to the actually-hidden ones — not silently empty.
func TestCollection_ResolveHonorsHiddenSelector(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Targets: []domain.Target{
		{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"},
		{ID: "aws:eks-2", ProviderID: "aws", Name: "eks-staging"},
	}}
	store.hidden = []domain.TargetID{"aws:eks-1"}
	store.collections = []domain.Collection{{
		ID:       "c",
		Name:     "hidden-ones",
		Selector: domain.LabelSelector{MatchLabels: map[string]string{"hidden": "true"}},
	}}

	members, err := NewCollectionService(store, nil).Resolve("hidden-ones")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(members) != 1 || members[0].ID != "aws:eks-1" {
		t.Fatalf("expected the hidden target, got %v", targetIDs(members))
	}
}

func TestCollection_Delete(t *testing.T) {
	store := storeWithTargets(prodAndLab()...)
	svc := NewCollectionService(store, nil)
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if _, err := svc.Create("production", "", sel, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete("production"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get("production"); err == nil {
		t.Error("expected collection to be gone after delete")
	}
	if err := svc.Delete("production"); err == nil {
		t.Error("expected error deleting a missing collection")
	}
}
