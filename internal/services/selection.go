package services

import (
	"fmt"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// SelectionService records the operator's current target or collection choice.
//
// For milestone 1 "use" is recorded intent: it persists the selection so the
// CLI can show "what am I pointed at". Materializing credentials into a live
// kubeconfig (az aks get-credentials / aws eks update-kubeconfig) is a
// provider action reserved for a later capability slice — kept out of here so
// selection stays provider-agnostic.
type SelectionService struct {
	store cache.CacheStore
	now   func() time.Time
}

// NewSelectionService builds a SelectionService. A nil now defaults to
// time.Now().UTC().
func NewSelectionService(store cache.CacheStore, now func() time.Time) *SelectionService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SelectionService{store: store, now: now}
}

// UseTarget records a target selection after verifying the target exists.
func (s *SelectionService) UseTarget(id domain.TargetID) (domain.Target, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return domain.Target{}, err
	}
	var found *domain.Target
	for i := range snap.Targets {
		if snap.Targets[i].ID == id {
			found = &snap.Targets[i]
			break
		}
	}
	if found == nil {
		return domain.Target{}, fmt.Errorf("target %q not found", id)
	}
	if err := s.store.SaveSelection(domain.Selection{TargetID: id, UpdatedAt: s.now()}); err != nil {
		return domain.Target{}, err
	}
	return *found, nil
}

// UseCollection records a collection selection.
func (s *SelectionService) UseCollection(id domain.CollectionID) error {
	return s.store.SaveSelection(domain.Selection{CollectionID: id, UpdatedAt: s.now()})
}

// Current returns the persisted selection.
func (s *SelectionService) Current() (domain.Selection, error) {
	return s.store.LoadSelection()
}
