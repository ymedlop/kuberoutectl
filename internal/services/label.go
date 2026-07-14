package services

import (
	"fmt"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// LabelService owns user-label mutation. The authoritative store is the
// separate user-labels state file; the snapshot's per-target UserLabels is a
// denormalized copy kept in sync here so reads reflect a change without
// requiring a resync.
type LabelService struct {
	store cache.CacheStore
}

// NewLabelService builds a LabelService.
func NewLabelService(store cache.CacheStore) *LabelService {
	return &LabelService{store: store}
}

// Add validates and sets a user label on a target. Reserved-namespace keys are
// rejected by domain validation, so users can never write kuberoutectl.io/*.
func (s *LabelService) Add(targetID domain.TargetID, key, value string) error {
	if err := domain.ValidateUserLabel(key, value); err != nil {
		return err
	}
	snap, idx, err := s.loadAndLocate(targetID)
	if err != nil {
		return err
	}

	labels, err := s.store.LoadUserLabels()
	if err != nil {
		return fmt.Errorf("load user labels: %w", err)
	}
	if labels == nil {
		labels = map[domain.TargetID]map[string]string{}
	}
	if labels[targetID] == nil {
		labels[targetID] = map[string]string{}
	}
	labels[targetID][key] = value
	if err := s.store.SaveUserLabels(labels); err != nil {
		return fmt.Errorf("save user labels: %w", err)
	}

	if snap.Targets[idx].UserLabels == nil {
		snap.Targets[idx].UserLabels = map[string]string{}
	}
	snap.Targets[idx].UserLabels[key] = value
	return s.store.SaveSnapshot(snap)
}

// Remove deletes a user label key from a target. Removing a missing key is an
// error so scripts get clear feedback.
func (s *LabelService) Remove(targetID domain.TargetID, key string) error {
	snap, idx, err := s.loadAndLocate(targetID)
	if err != nil {
		return err
	}

	labels, err := s.store.LoadUserLabels()
	if err != nil {
		return fmt.Errorf("load user labels: %w", err)
	}
	if _, ok := labels[targetID][key]; !ok {
		return fmt.Errorf("target %q has no user label %q", targetID, key)
	}
	delete(labels[targetID], key)
	if len(labels[targetID]) == 0 {
		delete(labels, targetID) // keep the state file tidy
	}
	if err := s.store.SaveUserLabels(labels); err != nil {
		return fmt.Errorf("save user labels: %w", err)
	}

	delete(snap.Targets[idx].UserLabels, key)
	return s.store.SaveSnapshot(snap)
}

// List returns the user labels for a target (authoritative state file).
func (s *LabelService) List(targetID domain.TargetID) (map[string]string, error) {
	labels, err := s.store.LoadUserLabels()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range labels[targetID] {
		out[k] = v
	}
	return out, nil
}

// loadAndLocate loads the snapshot and finds the target index, erroring if the
// target does not exist so label ops fail loudly on typos.
func (s *LabelService) loadAndLocate(targetID domain.TargetID) (domain.InventorySnapshot, int, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return domain.InventorySnapshot{}, -1, fmt.Errorf("load snapshot: %w", err)
	}
	for i := range snap.Targets {
		if snap.Targets[i].ID == targetID {
			return snap, i, nil
		}
	}
	return domain.InventorySnapshot{}, -1, fmt.Errorf("target %q not found (run `kuberoutectl sync` first)", targetID)
}
