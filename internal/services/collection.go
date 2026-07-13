package services

import (
	"fmt"
	"sort"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// CollectionService manages saved views over targets. A collection resolves to
// its selector matches unioned with any explicit static IDs, so newly
// discovered targets that match a selector join automatically on the next read
// — that is what makes collections saved views rather than static folders.
type CollectionService struct {
	store  cache.CacheStore
	engine *SelectorEngine
}

// NewCollectionService builds a CollectionService.
func NewCollectionService(store cache.CacheStore, engine *SelectorEngine) *CollectionService {
	if engine == nil {
		engine = NewSelectorEngine()
	}
	return &CollectionService{store: store, engine: engine}
}

// Create saves a new collection. Name must be non-empty and unique, and the
// definition must have at least one member source (selector or static IDs).
func (s *CollectionService) Create(name, description string, sel domain.LabelSelector, staticIDs []domain.TargetID) (domain.Collection, error) {
	if name == "" {
		return domain.Collection{}, fmt.Errorf("collection name must not be empty")
	}
	if sel.IsZero() && len(staticIDs) == 0 {
		return domain.Collection{}, fmt.Errorf("collection %q needs a selector or static targets", name)
	}
	cols, err := s.store.LoadCollections()
	if err != nil {
		return domain.Collection{}, fmt.Errorf("load collections: %w", err)
	}
	for _, c := range cols {
		if c.Name == name {
			return domain.Collection{}, fmt.Errorf("collection %q already exists", name)
		}
	}
	col := domain.Collection{
		ID:          domain.CollectionID(name),
		Name:        name,
		Description: description,
		Selector:    sel,
		StaticIDs:   staticIDs,
	}
	cols = append(cols, col)
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	if err := s.store.SaveCollections(cols); err != nil {
		return domain.Collection{}, fmt.Errorf("save collections: %w", err)
	}
	return col, nil
}

// List returns all saved collections, sorted by name.
func (s *CollectionService) List() ([]domain.Collection, error) {
	return s.store.LoadCollections()
}

// Get returns the collection with the given name.
func (s *CollectionService) Get(name string) (domain.Collection, error) {
	cols, err := s.store.LoadCollections()
	if err != nil {
		return domain.Collection{}, err
	}
	for _, c := range cols {
		if c.Name == name {
			return c, nil
		}
	}
	return domain.Collection{}, fmt.Errorf("collection %q not found", name)
}

// Delete removes a collection by name.
func (s *CollectionService) Delete(name string) error {
	cols, err := s.store.LoadCollections()
	if err != nil {
		return err
	}
	kept := cols[:0]
	found := false
	for _, c := range cols {
		if c.Name == name {
			found = true
			continue
		}
		kept = append(kept, c)
	}
	if !found {
		return fmt.Errorf("collection %q not found", name)
	}
	return s.store.SaveCollections(kept)
}

// Resolve returns the current members of a collection: selector matches unioned
// with static IDs, deduped and sorted by target ID for deterministic output.
// Static IDs that no longer exist in the inventory are silently skipped.
func (s *CollectionService) Resolve(name string) ([]domain.Target, error) {
	col, err := s.Get(name)
	if err != nil {
		return nil, err
	}
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	members := map[domain.TargetID]domain.Target{}
	if !col.Selector.IsZero() {
		for _, t := range s.engine.Filter(col.Selector, snap.Targets) {
			members[t.ID] = t
		}
	}
	if len(col.StaticIDs) > 0 {
		byID := map[domain.TargetID]domain.Target{}
		for _, t := range snap.Targets {
			byID[t.ID] = t
		}
		for _, id := range col.StaticIDs {
			if t, ok := byID[id]; ok {
				members[id] = t
			}
		}
	}

	out := make([]domain.Target, 0, len(members))
	for _, t := range members {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
