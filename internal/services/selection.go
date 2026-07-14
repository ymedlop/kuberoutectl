package services

import (
	"context"
	"fmt"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// SelectionService records the operator's current target or collection choice.
//
// "use" persists the selection so the CLI can show "what am I pointed at", and
// (optionally) materializes the target into the local kubeconfig through the
// provider's ContextActivator capability. Selection stays provider-agnostic;
// the kubeconfig side effect is delegated to the provider.
type SelectionService struct {
	store    cache.CacheStore
	registry *providers.Registry
	now      func() time.Time
}

// NewSelectionService builds a SelectionService. A nil now defaults to
// time.Now().UTC(). registry may be nil if activation is never requested.
func NewSelectionService(store cache.CacheStore, registry *providers.Registry, now func() time.Time) *SelectionService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SelectionService{store: store, registry: registry, now: now}
}

// UseTarget records a target selection after resolving ref (a full ID, alias,
// or name) to exactly one target. When activate is true it also fetches the
// target's credentials into the local kubeconfig via the owning provider
// (setting the current context). The selection is only recorded after a
// requested activation succeeds, so a failed kubeconfig fetch doesn't silently
// change "what am I pointed at".
func (s *SelectionService) UseTarget(ctx context.Context, ref string, activate bool) (domain.Target, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return domain.Target{}, err
	}
	AssignAliases(snap.Targets)
	found, err := ResolveTargetRef(snap.Targets, ref)
	if err != nil {
		return domain.Target{}, err
	}

	if activate {
		if err := s.activate(ctx, found); err != nil {
			return domain.Target{}, err
		}
	}

	if err := s.store.SaveSelection(domain.Selection{TargetID: found.ID, UpdatedAt: s.now()}); err != nil {
		return domain.Target{}, err
	}
	return found, nil
}

// activate materializes a target into the kubeconfig via its provider, gated on
// the CanSwitchContext capability and the ContextActivator interface.
func (s *SelectionService) activate(ctx context.Context, target domain.Target) error {
	if s.registry == nil {
		return fmt.Errorf("cannot update kubeconfig: no provider registry configured")
	}
	p, ok := s.registry.Get(target.ProviderID)
	if !ok {
		return fmt.Errorf("provider %q for target %q is not registered", target.ProviderID, target.ID)
	}
	if !p.Capabilities().CanSwitchContext {
		return fmt.Errorf("provider %q cannot write kubeconfig; re-run with --no-kubeconfig to record the selection only", target.ProviderID)
	}
	activator, ok := p.(providers.ContextActivator)
	if !ok {
		return fmt.Errorf("provider %q declares CanSwitchContext but does not implement activation", target.ProviderID)
	}
	return activator.Activate(ctx, target)
}

// UseCollection records a collection selection.
func (s *SelectionService) UseCollection(id domain.CollectionID) error {
	return s.store.SaveSelection(domain.Selection{CollectionID: id, UpdatedAt: s.now()})
}

// Current returns the persisted selection.
func (s *SelectionService) Current() (domain.Selection, error) {
	return s.store.LoadSelection()
}
