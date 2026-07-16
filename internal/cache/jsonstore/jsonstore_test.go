package jsonstore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	return New(filepath.Join(root, "cache"), filepath.Join(root, "state"))
}

func TestStore_MissingFilesReturnEmpty(t *testing.T) {
	s := newTestStore(t)

	snap, err := s.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot on empty: %v", err)
	}
	if len(snap.Targets) != 0 {
		t.Errorf("expected empty snapshot, got %d targets", len(snap.Targets))
	}

	labels, err := s.LoadUserLabels()
	if err != nil {
		t.Fatalf("LoadUserLabels on empty: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %d", len(labels))
	}

	cols, err := s.LoadCollections()
	if err != nil {
		t.Fatalf("LoadCollections on empty: %v", err)
	}
	if len(cols) != 0 {
		t.Errorf("expected no collections, got %d", len(cols))
	}
}

func TestStore_SnapshotRoundTrip(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	in := domain.InventorySnapshot{
		Targets: []domain.Target{{
			ID:           "t1",
			ProviderID:   "azure",
			Name:         "aks-prod",
			Health:       domain.HealthValid,
			ActionHint:   domain.ActionUse,
			SystemLabels: map[string]string{domain.LabelProvider: "azure"},
		}},
		SyncedAt: now,
	}
	if err := s.SaveSnapshot(in); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	out, err := s.LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(out.Targets) != 1 || out.Targets[0].ID != "t1" {
		t.Fatalf("round-trip targets mismatch: %+v", out.Targets)
	}
	if out.Targets[0].SystemLabels[domain.LabelProvider] != "azure" {
		t.Errorf("system label lost in round-trip")
	}
	if !out.SyncedAt.Equal(now) {
		t.Errorf("SyncedAt = %v, want %v", out.SyncedAt, now)
	}
}

func TestStore_UserLabelsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	in := map[domain.TargetID]map[string]string{
		"t1": {"env": "prod", "team": "platform"},
	}
	if err := s.SaveUserLabels(in); err != nil {
		t.Fatalf("SaveUserLabels: %v", err)
	}
	out, err := s.LoadUserLabels()
	if err != nil {
		t.Fatalf("LoadUserLabels: %v", err)
	}
	if out["t1"]["env"] != "prod" || out["t1"]["team"] != "platform" {
		t.Fatalf("user labels round-trip mismatch: %+v", out)
	}
}

// TestStore_SnapshotSaveDoesNotTouchUserState is the on-disk guarantee that a
// resync (SaveSnapshot) never overwrites user-owned labels.
func TestStore_SnapshotSaveDoesNotTouchUserState(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveUserLabels(map[domain.TargetID]map[string]string{"t1": {"env": "prod"}}); err != nil {
		t.Fatalf("SaveUserLabels: %v", err)
	}
	// Simulate a resync writing a fresh snapshot.
	if err := s.SaveSnapshot(domain.InventorySnapshot{Targets: []domain.Target{{ID: "t1"}}}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	labels, err := s.LoadUserLabels()
	if err != nil {
		t.Fatalf("LoadUserLabels: %v", err)
	}
	if labels["t1"]["env"] != "prod" {
		t.Fatalf("user label clobbered by snapshot save: %+v", labels)
	}
}

func TestStore_HiddenTargetsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	// Missing file reads as empty, not an error.
	if got, err := s.LoadHiddenTargets(); err != nil || len(got) != 0 {
		t.Fatalf("missing hidden file: got %v err %v", got, err)
	}
	want := []domain.TargetID{"aws:eks-1", "kubeconfig:context:homelab"}
	if err := s.SaveHiddenTargets(want); err != nil {
		t.Fatalf("SaveHiddenTargets: %v", err)
	}
	got, err := s.LoadHiddenTargets()
	if err != nil {
		t.Fatalf("LoadHiddenTargets: %v", err)
	}
	if len(got) != 2 || got[0] != "aws:eks-1" || got[1] != "kubeconfig:context:homelab" {
		t.Fatalf("round-trip mismatch: %v", got)
	}
}

// Hidden targets are user-owned: a resync (SaveSnapshot) must never touch them.
func TestStore_SnapshotSaveDoesNotTouchHiddenTargets(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveHiddenTargets([]domain.TargetID{"aws:eks-1"}); err != nil {
		t.Fatalf("SaveHiddenTargets: %v", err)
	}
	if err := s.SaveSnapshot(domain.InventorySnapshot{Targets: []domain.Target{{ID: "aws:eks-1"}}}); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	got, err := s.LoadHiddenTargets()
	if err != nil {
		t.Fatalf("LoadHiddenTargets: %v", err)
	}
	if len(got) != 1 || got[0] != "aws:eks-1" {
		t.Fatalf("hidden set clobbered by snapshot save: %v", got)
	}
}

func TestStore_SelectionRoundTrip(t *testing.T) {
	s := newTestStore(t)
	in := domain.Selection{TargetID: "t1", UpdatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.SaveSelection(in); err != nil {
		t.Fatalf("SaveSelection: %v", err)
	}
	out, err := s.LoadSelection()
	if err != nil {
		t.Fatalf("LoadSelection: %v", err)
	}
	if out.TargetID != "t1" {
		t.Errorf("selection round-trip mismatch: %+v", out)
	}
}
