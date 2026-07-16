package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestVisibilityService_HideUnhideByRef(t *testing.T) {
	store := seededTargetStore()
	vis := NewVisibilityService(store)

	found, err := vis.HideRef("eks-prod")
	if err != nil {
		t.Fatalf("HideRef: %v", err)
	}
	if found.ID != "aws:eks-1" {
		t.Fatalf("hid wrong target: %s", found.ID)
	}
	if ids, _ := store.LoadHiddenTargets(); len(ids) != 1 || ids[0] != "aws:eks-1" {
		t.Fatalf("hidden set = %v", ids)
	}
	// Now the default list drops it.
	list, _ := NewTargetService(store).List(TargetFilter{})
	if contains(targetIDs(list), "aws:eks-1") {
		t.Errorf("hidden target still in default list")
	}

	// Unhide reverses it — including resolving the (still-cached) hidden target.
	if _, err := vis.UnhideRef("aws:eks-1"); err != nil {
		t.Fatalf("UnhideRef: %v", err)
	}
	if ids, _ := store.LoadHiddenTargets(); len(ids) != 0 {
		t.Fatalf("unhide did not clear the set: %v", ids)
	}
}

func TestVisibilityService_HideBySelectorIsBulkAndIdempotent(t *testing.T) {
	store := seededTargetStore()
	vis := NewVisibilityService(store)
	sel := domain.LabelSelector{MatchLabels: map[string]string{"provider": "aws"}}

	matched, err := vis.HideSelector(sel)
	if err != nil {
		t.Fatalf("HideSelector: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 aws targets hidden, got %d", len(matched))
	}
	// Idempotent: hiding again does not duplicate IDs.
	if _, err := vis.HideSelector(sel); err != nil {
		t.Fatalf("HideSelector again: %v", err)
	}
	ids, _ := store.LoadHiddenTargets()
	if len(ids) != 2 {
		t.Fatalf("hidden set should have 2 unique IDs, got %v", ids)
	}
	// azure target untouched, aws ones hidden.
	list, _ := NewTargetService(store).List(TargetFilter{})
	if !contains(targetIDs(list), "azure:aks-1") {
		t.Errorf("azure target wrongly hidden")
	}
	if contains(targetIDs(list), "aws:eks-1") || contains(targetIDs(list), "aws:eks-2") {
		t.Errorf("aws targets should be hidden: %v", targetIDs(list))
	}
}

func TestVisibilityService_UnhideBySelector(t *testing.T) {
	store := seededTargetStore()
	store.hidden = []domain.TargetID{"aws:eks-1", "aws:eks-2"}
	vis := NewVisibilityService(store)
	// Unhide everything currently hidden.
	sel := domain.LabelSelector{MatchLabels: map[string]string{"hidden": "true"}}
	matched, err := vis.UnhideSelector(sel)
	if err != nil {
		t.Fatalf("UnhideSelector: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched, got %d", len(matched))
	}
	if ids, _ := store.LoadHiddenTargets(); len(ids) != 0 {
		t.Fatalf("expected empty hidden set, got %v", ids)
	}
}

// A hidden ID for a target not in the snapshot persists harmlessly (it re-hides
// the target if it reappears via a later sync).
func TestVisibilityService_OrphanHiddenIDPersists(t *testing.T) {
	store := newMemStore()
	store.hidden = []domain.TargetID{"aws:ghost"}
	// No targets in the snapshot; listing must not choke.
	if _, err := NewTargetService(store).List(TargetFilter{}); err != nil {
		t.Fatalf("List with orphan hidden ID: %v", err)
	}
	if ids, _ := store.LoadHiddenTargets(); len(ids) != 1 {
		t.Fatalf("orphan hidden ID should persist: %v", ids)
	}
}
