package services

import (
	"sort"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// ApplyVisibility sets Hidden on each target whose ID is in the hidden set. It
// is the read-time join that makes hiding a computed-on-read property (like
// aliases), so it survives a resync without ever being stored in the snapshot.
// It must be called on every path that surfaces targets or runs the selector
// engine (list, collections, selection), so the visible/hidden selector keys are
// honest everywhere.
func ApplyVisibility(targets []domain.Target, hidden map[domain.TargetID]bool) {
	for i := range targets {
		if hidden[targets[i].ID] {
			targets[i].Hidden = true
		}
	}
}

// loadHiddenSet reads the user-owned hidden-target IDs as a lookup set.
func loadHiddenSet(store cache.CacheStore) (map[domain.TargetID]bool, error) {
	ids, err := store.LoadHiddenTargets()
	if err != nil {
		return nil, err
	}
	set := make(map[domain.TargetID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

// VisibilityService toggles the user-owned hidden-target set. Hiding is
// persistent and reversible: the set lives in user state and survives a resync,
// so a hidden target stays out of default listings until explicitly unhidden.
type VisibilityService struct{ store cache.CacheStore }

// NewVisibilityService builds a VisibilityService.
func NewVisibilityService(store cache.CacheStore) *VisibilityService {
	return &VisibilityService{store: store}
}

// resolvedTargets returns the snapshot's targets with aliases and visibility
// applied, so refs resolve like the read paths and selectors can match on
// visible/hidden.
func (s *VisibilityService) resolvedTargets() ([]domain.Target, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	targets := make([]domain.Target, len(snap.Targets))
	copy(targets, snap.Targets)
	AssignAliases(targets)
	hidden, err := loadHiddenSet(s.store)
	if err != nil {
		return nil, err
	}
	ApplyVisibility(targets, hidden)
	return targets, nil
}

// HideRef hides the single target matching ref (id/alias/name), returning it.
func (s *VisibilityService) HideRef(ref string) (domain.Target, error) {
	return s.toggleRef(ref, true)
}

// UnhideRef reveals the single target matching ref, returning it.
func (s *VisibilityService) UnhideRef(ref string) (domain.Target, error) {
	return s.toggleRef(ref, false)
}

func (s *VisibilityService) toggleRef(ref string, hide bool) (domain.Target, error) {
	targets, err := s.resolvedTargets()
	if err != nil {
		return domain.Target{}, err
	}
	found, err := ResolveTargetRef(targets, ref)
	if err != nil {
		return domain.Target{}, err
	}
	if err := s.setHidden([]domain.TargetID{found.ID}, hide); err != nil {
		return domain.Target{}, err
	}
	return found, nil
}

// HideSelector hides every target the selector matches, returning them.
func (s *VisibilityService) HideSelector(sel domain.LabelSelector) ([]domain.Target, error) {
	return s.toggleSelector(sel, true)
}

// UnhideSelector reveals every target the selector matches, returning them.
func (s *VisibilityService) UnhideSelector(sel domain.LabelSelector) ([]domain.Target, error) {
	return s.toggleSelector(sel, false)
}

func (s *VisibilityService) toggleSelector(sel domain.LabelSelector, hide bool) ([]domain.Target, error) {
	targets, err := s.resolvedTargets()
	if err != nil {
		return nil, err
	}
	matched := NewSelectorEngine().Filter(sel, targets)
	ids := make([]domain.TargetID, len(matched))
	for i, t := range matched {
		ids[i] = t.ID
	}
	if err := s.setHidden(ids, hide); err != nil {
		return nil, err
	}
	return matched, nil
}

// setHidden adds or removes ids in the persisted hidden set, writing it back as
// a sorted, de-duplicated slice for deterministic output.
func (s *VisibilityService) setHidden(ids []domain.TargetID, hide bool) error {
	current, err := s.store.LoadHiddenTargets()
	if err != nil {
		return err
	}
	set := make(map[domain.TargetID]bool, len(current))
	for _, id := range current {
		set[id] = true
	}
	for _, id := range ids {
		if hide {
			set[id] = true
		} else {
			delete(set, id)
		}
	}
	out := make([]domain.TargetID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return s.store.SaveHiddenTargets(out)
}
