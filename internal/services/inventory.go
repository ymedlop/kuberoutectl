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

func (s *CredentialService) List() ([]domain.Credential, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Credentials, nil
}

func (s *CredentialService) Get(id domain.CredentialID) (domain.Credential, error) {
	creds, err := s.List()
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

func (s *TargetService) List() ([]domain.Target, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Targets, nil
}

func (s *TargetService) Get(id domain.TargetID) (domain.Target, error) {
	targets, err := s.List()
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
