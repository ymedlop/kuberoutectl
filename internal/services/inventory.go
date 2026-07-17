package services

import (
	"context"
	"fmt"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// The read services below are thin projections over the cached snapshot. They
// keep the CLI free of persistence details and give each inventory noun a
// single, testable access point. They are grouped in one file because they
// share the same trivial shape; split them if any grows real logic.

// SourceService lists discovered access sources.
type SourceService struct{ store cache.CacheStore }

func NewSourceService(store cache.CacheStore) *SourceService { return &SourceService{store: store} }

func (s *SourceService) List() ([]domain.AccessSource, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Sources, nil
}

// ScopeService lists discovered scopes (e.g. Azure subscriptions).
type ScopeService struct{ store cache.CacheStore }

func NewScopeService(store cache.CacheStore) *ScopeService { return &ScopeService{store: store} }

func (s *ScopeService) List() ([]domain.Scope, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Scopes, nil
}

// CredentialService lists/inspects credentials and drives renewal through the
// owning provider, gated on that provider's capabilities.
type CredentialService struct {
	store    cache.CacheStore
	registry *providers.Registry
}

func NewCredentialService(store cache.CacheStore, reg *providers.Registry) *CredentialService {
	return &CredentialService{store: store, registry: reg}
}

// List returns credentials, optionally narrowed to one provider. An empty
// provider matches everything, mirroring TargetFilter.Provider.
func (s *CredentialService) List(provider domain.ProviderID) ([]domain.Credential, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	if provider == "" {
		return snap.Credentials, nil
	}
	kept := make([]domain.Credential, 0, len(snap.Credentials))
	for _, c := range snap.Credentials {
		if c.ProviderID == provider {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

func (s *CredentialService) Get(id domain.CredentialID) (domain.Credential, error) {
	creds, err := s.List("")
	if err != nil {
		return domain.Credential{}, err
	}
	for _, c := range creds {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Credential{}, fmt.Errorf("credential %q not found", id)
}

// Renew looks up the credential, checks the owning provider supports renewal,
// then delegates. Capability gating lives here so the CLI never assumes every
// provider can renew.
func (s *CredentialService) Renew(ctx context.Context, id domain.CredentialID) error {
	cred, err := s.Get(id)
	if err != nil {
		return err
	}
	p, ok := s.registry.Get(cred.ProviderID)
	if !ok {
		return fmt.Errorf("provider %q for credential %q is not registered", cred.ProviderID, id)
	}
	if !p.Capabilities().CanRenew {
		return fmt.Errorf("provider %q does not support renew", cred.ProviderID)
	}
	return p.Renew(ctx, cred)
}

// TargetService lists and inspects Kubernetes targets.
type TargetService struct{ store cache.CacheStore }

func NewTargetService(store cache.CacheStore) *TargetService { return &TargetService{store: store} }

// TargetFilter narrows a target listing. A zero value matches everything except
// hidden targets (see IncludeHidden).
type TargetFilter struct {
	// Provider, when non-empty, keeps only targets from that provider.
	Provider domain.ProviderID
	// Selector, when non-nil, keeps only targets the selector matches
	// (evaluated against SelectionLabels, like collections).
	Selector *domain.LabelSelector
	// IncludeHidden keeps user-hidden targets in the result. By default they are
	// dropped, unless Selector already constrains visibility (visible/hidden).
	IncludeHidden bool
}

// all loads the snapshot's targets, as a fresh copy, with aliases assigned.
// Every read path goes through here so aliases are consistent between list,
// inspect, and use. It copies rather than returning the store's backing slice
// so filtering/aliasing can never mutate the cached snapshot.
func (s *TargetService) all() ([]domain.Target, error) {
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

// List returns targets matching the filter, preserving snapshot order. Hidden
// targets are dropped unless the filter opts in (IncludeHidden) or its selector
// already constrains visibility.
func (s *TargetService) List(f TargetFilter) ([]domain.Target, error) {
	targets, err := s.all()
	if err != nil {
		return nil, err
	}
	if f.Provider != "" {
		kept := make([]domain.Target, 0, len(targets))
		for _, t := range targets {
			if t.ProviderID == f.Provider {
				kept = append(kept, t)
			}
		}
		targets = kept
	}
	if f.Selector != nil {
		targets = NewSelectorEngine().Filter(*f.Selector, targets)
	}
	if !f.IncludeHidden && !selectorConstrainsVisibility(f.Selector) {
		kept := make([]domain.Target, 0, len(targets))
		for _, t := range targets {
			if !t.Hidden {
				kept = append(kept, t)
			}
		}
		targets = kept
	}
	return targets, nil
}

// selectorConstrainsVisibility reports whether the selector already filters on a
// visibility key, in which case List must not additionally auto-drop hidden
// targets (otherwise `-l hidden=true` would return nothing).
func selectorConstrainsVisibility(sel *domain.LabelSelector) bool {
	return sel != nil && (sel.HasKey("hidden") || sel.HasKey("visible"))
}

// Get returns a single target by its exact ID.
func (s *TargetService) Get(id domain.TargetID) (domain.Target, error) {
	targets, err := s.all()
	if err != nil {
		return domain.Target{}, err
	}
	for _, t := range targets {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.Target{}, fmt.Errorf("target %q not found", id)
}

// Resolve returns a single target by a flexible reference: full ID, alias, or
// name (see ResolveTargetRef). This is what lets the CLI accept short handles
// wherever a target ID is expected.
func (s *TargetService) Resolve(ref string) (domain.Target, error) {
	targets, err := s.all()
	if err != nil {
		return domain.Target{}, err
	}
	return ResolveTargetRef(targets, ref)
}

// Delete removes the target matching ref (id, alias, or name) from the cached
// snapshot and persists, returning the removed target. Only the target is
// dropped — its scope, credential, and source are left in place. This is a cache
// cleanup, not a permanent exclusion: a later `sync` of the owning provider
// re-adds the target if the cluster still exists.
func (s *TargetService) Delete(ref string) (domain.Target, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return domain.Target{}, err
	}
	// Resolve against an aliased copy so ref accepts id/alias/name, exactly like
	// the read paths, without mutating the snapshot we are about to save.
	resolved := make([]domain.Target, len(snap.Targets))
	copy(resolved, snap.Targets)
	AssignAliases(resolved)
	found, err := ResolveTargetRef(resolved, ref)
	if err != nil {
		return domain.Target{}, err
	}
	kept := make([]domain.Target, 0, len(snap.Targets))
	for _, t := range snap.Targets {
		if t.ID != found.ID {
			kept = append(kept, t)
		}
	}
	snap.Targets = kept
	if err := s.store.SaveSnapshot(snap); err != nil {
		return domain.Target{}, err
	}
	return found, nil
}

// Clear removes all targets from the cached snapshot and persists, returning the
// number removed. Scopes, credentials, and sources are left intact; a resync
// repopulates targets.
func (s *TargetService) Clear() (int, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return 0, err
	}
	n := len(snap.Targets)
	snap.Targets = nil
	if err := s.store.SaveSnapshot(snap); err != nil {
		return 0, err
	}
	return n, nil
}
