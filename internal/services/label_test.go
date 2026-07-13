package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func storeWithTarget(id domain.TargetID) *memStore {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{Targets: []domain.Target{{ID: id, Name: string(id)}}}
	return m
}

func TestLabel_AddPersistsToBothStoreAndSnapshot(t *testing.T) {
	store := storeWithTarget("t1")
	svc := NewLabelService(store)

	if err := svc.Add("t1", "env", "prod"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Authoritative state file.
	if store.userLabels["t1"]["env"] != "prod" {
		t.Errorf("user-labels store not updated: %+v", store.userLabels)
	}
	// Denormalized snapshot copy, so reads are consistent without a resync.
	if store.snap.Targets[0].UserLabels["env"] != "prod" {
		t.Errorf("snapshot copy not updated: %+v", store.snap.Targets[0].UserLabels)
	}
}

func TestLabel_AddRejectsReservedNamespace(t *testing.T) {
	svc := NewLabelService(storeWithTarget("t1"))
	if err := svc.Add("t1", domain.LabelProvider, "azure"); err == nil {
		t.Fatal("expected reserved-namespace label to be rejected")
	}
}

func TestLabel_AddUnknownTarget(t *testing.T) {
	svc := NewLabelService(storeWithTarget("t1"))
	if err := svc.Add("ghost", "env", "prod"); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestLabel_RemoveMissingKeyErrors(t *testing.T) {
	svc := NewLabelService(storeWithTarget("t1"))
	if err := svc.Remove("t1", "env"); err == nil {
		t.Fatal("expected error removing a label that does not exist")
	}
}

func TestLabel_AddThenRemoveThenList(t *testing.T) {
	store := storeWithTarget("t1")
	svc := NewLabelService(store)
	if err := svc.Add("t1", "env", "prod"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.Add("t1", "team", "platform"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.Remove("t1", "env"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	labels, err := svc.List("t1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(labels) != 1 || labels["team"] != "platform" {
		t.Errorf("unexpected labels after remove: %+v", labels)
	}
	// Snapshot copy reflects the removal too.
	if _, ok := store.snap.Targets[0].UserLabels["env"]; ok {
		t.Errorf("removed label still present in snapshot copy")
	}
}
