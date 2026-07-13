package services

import (
	"context"
	"fmt"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// DiscoveryService coordinates a single-provider sync and persists the result.
// It owns the two rules that keep user organization safe across resyncs:
//
//  1. Syncing one provider replaces only that provider's inventory, leaving
//     other providers' discovered state intact.
//  2. User labels (stored separately) are re-attached to freshly discovered
//     targets by ID, so discovery never overwrites them.
type DiscoveryService struct {
	registry *providers.Registry
	store    cache.CacheStore
	now      func() time.Time
}

// NewDiscoveryService builds a DiscoveryService. If now is nil it defaults to
// time.Now().UTC().
func NewDiscoveryService(reg *providers.Registry, store cache.CacheStore, now func() time.Time) *DiscoveryService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &DiscoveryService{registry: reg, store: store, now: now}
}

// Sync discovers inventory for one provider, merges it into the snapshot, and
// persists. It returns the merged snapshot.
func (s *DiscoveryService) Sync(ctx context.Context, providerID domain.ProviderID) (domain.InventorySnapshot, error) {
	p, ok := s.registry.Get(providerID)
	if !ok {
		return domain.InventorySnapshot{}, fmt.Errorf("provider %q is not registered", providerID)
	}

	res, err := p.Discover(ctx, providers.DiscoveryInput{})
	if err != nil {
		return domain.InventorySnapshot{}, fmt.Errorf("discover %q: %w", providerID, err)
	}

	prior, err := s.store.LoadSnapshot()
	if err != nil {
		return domain.InventorySnapshot{}, fmt.Errorf("load snapshot: %w", err)
	}

	merged := mergeProviderResult(prior, providerID, res, s.now())

	userLabels, err := s.store.LoadUserLabels()
	if err != nil {
		return domain.InventorySnapshot{}, fmt.Errorf("load user labels: %w", err)
	}
	applyUserLabels(merged.Targets, userLabels)

	if err := s.store.SaveSnapshot(merged); err != nil {
		return domain.InventorySnapshot{}, fmt.Errorf("save snapshot: %w", err)
	}
	return merged, nil
}

// mergeProviderResult drops all prior entities belonging to providerID and
// appends the freshly discovered ones, preserving other providers' data.
func mergeProviderResult(prior domain.InventorySnapshot, providerID domain.ProviderID, res providers.DiscoveryResult, now time.Time) domain.InventorySnapshot {
	out := domain.InventorySnapshot{SyncedAt: now}

	for _, s := range prior.Sources {
		if s.ProviderID != providerID {
			out.Sources = append(out.Sources, s)
		}
	}
	out.Sources = append(out.Sources, res.Sources...)

	for _, c := range prior.Credentials {
		if c.ProviderID != providerID {
			out.Credentials = append(out.Credentials, c)
		}
	}
	out.Credentials = append(out.Credentials, res.Credentials...)

	for _, sc := range prior.Scopes {
		if sc.ProviderID != providerID {
			out.Scopes = append(out.Scopes, sc)
		}
	}
	out.Scopes = append(out.Scopes, res.Scopes...)

	for _, t := range prior.Targets {
		if t.ProviderID != providerID {
			out.Targets = append(out.Targets, t)
		}
	}
	out.Targets = append(out.Targets, res.Targets...)

	return out
}

// applyUserLabels copies stored user labels onto targets by ID. Providers
// return targets with empty UserLabels; this is the single place they are
// populated, which is what makes them survive a resync.
func applyUserLabels(targets []domain.Target, userLabels map[domain.TargetID]map[string]string) {
	for i := range targets {
		labels, ok := userLabels[targets[i].ID]
		if !ok || len(labels) == 0 {
			continue
		}
		cp := make(map[string]string, len(labels))
		for k, v := range labels {
			cp[k] = v
		}
		targets[i].UserLabels = cp
	}
}
