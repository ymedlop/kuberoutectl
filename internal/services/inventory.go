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
// TargetService reads and organizes the target inventory. The registry is only
// needed for the live access check (ResolveWithAccessCheck) and may be nil —
// every other method is a pure cache projection and makes no external call.
type TargetService struct {
	store    cache.CacheStore
	registry *providers.Registry
}

func NewTargetService(store cache.CacheStore, reg *providers.Registry) *TargetService {
	return &TargetService{store: store, registry: reg}
}

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
	return s.decorate(snap)
}

// decorate turns a loaded snapshot's targets into the read-time view: a fresh
// copy with aliases and visibility applied. Split out of all() so callers that
// need the snapshot's credentials too can share one read with it rather than
// loading the file again.
func (s *TargetService) decorate(snap domain.InventorySnapshot) ([]domain.Target, error) {
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
	targets = applyTargetFilter(targets, f)
	return targets, nil
}

// applyTargetFilter narrows a decorated target set. Shared by List and
// ListWithCredentials so the two can never disagree about what a filter means.
func applyTargetFilter(targets []domain.Target, f TargetFilter) []domain.Target {
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
	return targets
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

// TargetWithCredentials pairs a target with the credentials that can reach it,
// primary first. Health per credential is joined from the snapshot on read
// rather than copied onto the target, so there is no second copy to drift —
// the same rule the target's own Health follows.
type TargetWithCredentials struct {
	Target      domain.Target       `json:"target"`
	Credentials []domain.Credential `json:"credentials"`
	// AccessReason explains why a live check could not conclude, and is empty
	// both when it did and when none was asked for.
	AccessReason string `json:"access_reason,omitempty"`
}

// CredentialNames renders the reaching credentials for display, primary first.
func (t TargetWithCredentials) CredentialNames() []string {
	out := make([]string, 0, len(t.Credentials))
	for _, c := range t.Credentials {
		out = append(out, c.Name)
	}
	return out
}

// ResolveWithCredentials resolves a target reference and joins in every
// credential that can reach it.
//
// The snapshot is read once and both halves derive from it. Loading separately
// for the target and for the credentials would let a concurrent `sync` land
// between the two reads, joining one generation's targets against another's
// credentials.
func (s *TargetService) ResolveWithCredentials(ref string) (TargetWithCredentials, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return TargetWithCredentials{}, err
	}
	targets, err := s.decorate(snap)
	if err != nil {
		return TargetWithCredentials{}, err
	}
	t, err := ResolveTargetRef(targets, ref)
	if err != nil {
		return TargetWithCredentials{}, err
	}
	return TargetWithCredentials{Target: t, Credentials: credentialsFor(t, snap.Credentials)}, nil
}

// ListWithCredentials is List plus the credential join, for callers that need
// to show how each target is reached. Same single-read rule as
// ResolveWithCredentials.
func (s *TargetService) ListWithCredentials(f TargetFilter) ([]TargetWithCredentials, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	targets, err := s.decorate(snap)
	if err != nil {
		return nil, err
	}
	targets = applyTargetFilter(targets, f)

	out := make([]TargetWithCredentials, 0, len(targets))
	for _, t := range targets {
		out = append(out, TargetWithCredentials{Target: t, Credentials: credentialsFor(t, snap.Credentials)})
	}
	return out, nil
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

// checkAccess asks a target's provider, live, which of the given credentials it
// admits from inside.
//
// One function rather than one per caller: `target use` and `target inspect`
// need the same call with the same inputs, and two entry points is how their
// answers eventually stop agreeing.
//
// Every "cannot answer" collapses to the zero AccessCheck — no registry, no such
// provider, a provider without the capability, or a provider that rejected the
// target. Callers therefore never branch on provider identity, which is what
// keeps this layer provider-agnostic, and "not attempted" and "this provider has
// no such concept" are deliberately the same value: neither tells you anything
// about the target.
func checkAccess(ctx context.Context, reg *providers.Registry, t domain.Target, creds []domain.Credential) providers.AccessCheck {
	if reg == nil {
		return providers.AccessCheck{}
	}
	p, ok := reg.Get(t.ProviderID)
	if !ok {
		return providers.AccessCheck{}
	}
	checker, ok := p.(providers.AccessChecker)
	if !ok {
		return providers.AccessCheck{}
	}
	res, err := checker.CheckAccess(ctx, t, creds)
	if err != nil {
		// A provider signalling a caller mistake. Surfaced as a reason rather than
		// propagated: no command should fail because a diagnostic could not be
		// produced.
		return providers.AccessCheck{Reason: err.Error()}
	}
	return res
}

// ResolveWithAccessCheck is ResolveWithCredentials plus a live operability
// check, replacing whatever the last sync established.
//
// A separate method rather than a boolean on ResolveWithCredentials: a bare
// `true` at a call site says nothing about what it selects, and these two differ
// in whether they touch the network — which is exactly the kind of thing a
// reader should not have to look up.
func (s *TargetService) ResolveWithAccessCheck(ctx context.Context, ref string) (TargetWithCredentials, error) {
	joined, err := s.ResolveWithCredentials(ref)
	if err != nil {
		return TargetWithCredentials{}, err
	}
	live := checkAccess(ctx, s.registry, joined.Target, joined.Credentials)
	if live.Mode == "" && live.Reason == "" {
		return joined, nil // nothing was attempted; keep what the sync knew
	}
	// Override rather than merge: two sources for one answer is how they drift.
	joined.Target.AccessCheck = live.Mode
	joined.Target.OperableCredentialIDs = live.Operable
	joined.AccessReason = live.Reason
	return joined, nil
}
