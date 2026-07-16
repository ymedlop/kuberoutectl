package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func seededTargetStore() *memStore {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Sources:     []domain.AccessSource{{ID: "aws:source", ProviderID: "aws"}},
		Credentials: []domain.Credential{{ID: "aws:cred", ProviderID: "aws"}},
		Scopes:      []domain.Scope{{ID: "aws:scope", ProviderID: "aws"}},
		Targets: []domain.Target{
			{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"},
			{ID: "aws:eks-2", ProviderID: "aws", Name: "eks-staging"},
			{ID: "azure:aks-1", ProviderID: "azure", Name: "aks-prod"},
		},
	}
	return store
}

func TestTargetService_Delete_ByName(t *testing.T) {
	store := seededTargetStore()
	removed, err := NewTargetService(store).Delete("eks-prod")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if removed.ID != "aws:eks-1" {
		t.Fatalf("removed wrong target: %s", removed.ID)
	}
	got := targetIDs(store.snap.Targets)
	if contains(got, "aws:eks-1") {
		t.Errorf("deleted target still present: %v", got)
	}
	if !contains(got, "aws:eks-2") || !contains(got, "azure:aks-1") {
		t.Errorf("other targets dropped: %v", got)
	}
	// Scopes/credentials/sources are untouched by a target delete.
	if len(store.snap.Scopes) != 1 || len(store.snap.Credentials) != 1 || len(store.snap.Sources) != 1 {
		t.Errorf("delete must not touch scopes/creds/sources")
	}
}

func TestTargetService_Delete_ByID(t *testing.T) {
	store := seededTargetStore()
	removed, err := NewTargetService(store).Delete("azure:aks-1")
	if err != nil {
		t.Fatalf("Delete by id: %v", err)
	}
	if removed.Name != "aks-prod" {
		t.Fatalf("resolved wrong target: %s", removed.Name)
	}
	if len(store.snap.Targets) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(store.snap.Targets))
	}
}

func TestTargetService_Delete_NotFound(t *testing.T) {
	store := seededTargetStore()
	if _, err := NewTargetService(store).Delete("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown ref")
	}
	if len(store.snap.Targets) != 3 {
		t.Errorf("failed delete must not mutate the snapshot")
	}
}

// An ambiguous name (two targets share it) must error via ResolveTargetRef and
// leave the snapshot untouched — never delete an arbitrary one.
func TestTargetService_Delete_AmbiguousName(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Targets: []domain.Target{
		{ID: "aws:dup-1", ProviderID: "aws", Name: "dup"},
		{ID: "aws:dup-2", ProviderID: "aws", Name: "dup"},
	}}
	if _, err := NewTargetService(store).Delete("dup"); err == nil {
		t.Fatal("expected error deleting an ambiguous name")
	}
	if len(store.snap.Targets) != 2 {
		t.Errorf("ambiguous delete must not mutate the snapshot")
	}
}

func TestTargetService_Delete_PreservesOrder(t *testing.T) {
	store := seededTargetStore()
	if _, err := NewTargetService(store).Delete("eks-staging"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got := targetIDs(store.snap.Targets)
	want := []string{"aws:eks-1", "azure:aks-1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("order not preserved: got %v want %v", got, want)
	}
}

func TestApplyVisibility(t *testing.T) {
	targets := []domain.Target{{ID: "a"}, {ID: "b"}}
	ApplyVisibility(targets, map[domain.TargetID]bool{"b": true})
	if targets[0].Hidden {
		t.Error("a should be visible")
	}
	if !targets[1].Hidden {
		t.Error("b should be hidden")
	}
}

func TestTargetService_List_HidesHiddenByDefault(t *testing.T) {
	store := seededTargetStore()
	store.hidden = []domain.TargetID{"aws:eks-1"}
	svc := NewTargetService(store)

	def, err := svc.List(TargetFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if contains(targetIDs(def), "aws:eks-1") {
		t.Errorf("hidden target shown by default: %v", targetIDs(def))
	}
	if !contains(targetIDs(def), "aws:eks-2") {
		t.Errorf("visible target missing: %v", targetIDs(def))
	}

	all, err := svc.List(TargetFilter{IncludeHidden: true})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if !contains(targetIDs(all), "aws:eks-1") {
		t.Errorf("IncludeHidden must show hidden: %v", targetIDs(all))
	}

	// Resolve/Get still see a hidden target by ref (so use/inspect/unhide work).
	if _, err := svc.Resolve("aws:eks-1"); err != nil {
		t.Errorf("Resolve must see hidden target: %v", err)
	}
}

// Tripwire for the "Hidden is computed-on-read, never persisted" invariant:
// reading marks targets hidden in the returned copy, but the stored snapshot
// must never carry a computed Hidden=true (all() operates on a copy).
func TestTargetService_HiddenNotPersistedToSnapshot(t *testing.T) {
	store := seededTargetStore()
	store.hidden = []domain.TargetID{"aws:eks-1"}

	got, err := NewTargetService(store).List(TargetFilter{IncludeHidden: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var markedOnRead bool
	for _, tg := range got {
		if tg.ID == "aws:eks-1" && tg.Hidden {
			markedOnRead = true
		}
	}
	if !markedOnRead {
		t.Fatal("expected the hidden target to be marked Hidden in the read result")
	}

	snap, _ := store.LoadSnapshot()
	for _, tg := range snap.Targets {
		if tg.Hidden {
			t.Errorf("computed Hidden must not be persisted into the snapshot: %s", tg.ID)
		}
	}
}

func TestTargetService_List_HiddenSelectorIsolatesHidden(t *testing.T) {
	store := seededTargetStore()
	store.hidden = []domain.TargetID{"aws:eks-1"}
	sel := domain.LabelSelector{MatchLabels: map[string]string{"hidden": "true"}}
	got, err := NewTargetService(store).List(TargetFilter{Selector: &sel})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "aws:eks-1" {
		t.Fatalf("expected only the hidden target, got %v", targetIDs(got))
	}
}

func TestTargetService_List_NonVisibilitySelectorStillDropsHidden(t *testing.T) {
	store := seededTargetStore()
	store.hidden = []domain.TargetID{"aws:eks-1"}
	sel := domain.LabelSelector{MatchLabels: map[string]string{"provider": "aws"}}
	got, err := NewTargetService(store).List(TargetFilter{Selector: &sel})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if contains(targetIDs(got), "aws:eks-1") {
		t.Errorf("a non-visibility selector must still drop hidden: %v", targetIDs(got))
	}
	if !contains(targetIDs(got), "aws:eks-2") {
		t.Errorf("visible aws target missing: %v", targetIDs(got))
	}
}

func TestTargetService_Clear(t *testing.T) {
	store := seededTargetStore()
	n, err := NewTargetService(store).Clear()
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 cleared, got %d", n)
	}
	if len(store.snap.Targets) != 0 {
		t.Errorf("targets not cleared: %v", targetIDs(store.snap.Targets))
	}
	// Targets-only: everything else survives.
	if len(store.snap.Scopes) != 1 || len(store.snap.Credentials) != 1 || len(store.snap.Sources) != 1 {
		t.Errorf("clear must wipe targets only, not scopes/creds/sources")
	}
}

func TestTargetService_Clear_Empty(t *testing.T) {
	store := newMemStore()
	n, err := NewTargetService(store).Clear()
	if err != nil {
		t.Fatalf("Clear on empty: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

// Ripple: deleting a target that a collection references as a static member must
// not break collection resolution — it is skipped, like a resync removal.
func TestTargetService_Delete_CollectionStaticMemberTolerated(t *testing.T) {
	store := seededTargetStore()
	store.collections = []domain.Collection{{
		ID:        "c1",
		Name:      "keep",
		StaticIDs: []domain.TargetID{"aws:eks-1", "azure:aks-1"},
	}}
	if _, err := NewTargetService(store).Delete("aws:eks-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	members, err := NewCollectionService(store, nil).Resolve("keep")
	if err != nil {
		t.Fatalf("Resolve after delete: %v", err)
	}
	ids := targetIDs(members)
	if contains(ids, "aws:eks-1") {
		t.Errorf("deleted static member should be skipped: %v", ids)
	}
	if !contains(ids, "azure:aks-1") {
		t.Errorf("surviving static member missing: %v", ids)
	}
}

// Ripple: deleting the currently-selected target leaves a stale selection that
// Status surfaces (nil Target), not an error.
func TestTargetService_Delete_StaleSelectionTolerated(t *testing.T) {
	store := seededTargetStore()
	_ = store.SaveSelection(domain.Selection{TargetID: "aws:eks-1"})
	if _, err := NewTargetService(store).Delete("aws:eks-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	st, err := NewSelectionService(store, nil, nil).Status()
	if err != nil {
		t.Fatalf("Status after delete must not error: %v", err)
	}
	if st.Target != nil {
		t.Errorf("expected stale selection (nil Target), got %v", st.Target)
	}
	if st.Selection.TargetID != "aws:eks-1" {
		t.Errorf("selection should still record what it pointed at")
	}
}
