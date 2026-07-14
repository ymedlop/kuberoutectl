package services

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// staticProvider models a provider that cannot renew (e.g. future kubeconfig).
type staticProvider struct{ id domain.ProviderID }

func (s staticProvider) ID() domain.ProviderID { return s.id }
func (s staticProvider) Capabilities() domain.Capabilities {
	return domain.Capabilities{StaticCredentials: true, CanRenew: false}
}
func (s staticProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return providers.DiscoveryResult{}, nil
}
func (s staticProvider) Renew(context.Context, domain.Credential) error {
	return providers.ErrUnsupported
}

func TestCredential_ListAndGet(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Credentials: []domain.Credential{
		{ID: "c1", ProviderID: "azure", Identity: "yeray@example.com", Health: domain.HealthValid},
	}}
	svc := NewCredentialService(store, providers.NewRegistry())

	got, err := svc.Get("c1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Identity != "yeray@example.com" {
		t.Errorf("unexpected credential: %+v", got)
	}
	if _, err := svc.Get("missing"); err == nil {
		t.Error("expected error for missing credential")
	}
}

func TestCredential_ListFiltersByProvider(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Credentials: []domain.Credential{
		{ID: "c1", ProviderID: "azure"},
		{ID: "c2", ProviderID: "aws"},
		{ID: "c3", ProviderID: "aws"},
	}}
	svc := NewCredentialService(store, providers.NewRegistry())

	all, err := svc.List("")
	if err != nil || len(all) != 3 {
		t.Fatalf("no filter: got %d (%v), want 3", len(all), err)
	}
	awsOnly, err := svc.List("aws")
	if err != nil || len(awsOnly) != 2 {
		t.Fatalf("provider filter: got %d (%v), want 2", len(awsOnly), err)
	}
	for _, c := range awsOnly {
		if c.ProviderID != "aws" {
			t.Errorf("filter leaked %q", c.ProviderID)
		}
	}
}

func TestCredential_RenewGatedOnCapability(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Credentials: []domain.Credential{
		{ID: "kube-1", ProviderID: "kubeconfig"},
	}}
	reg := providers.NewRegistry()
	_ = reg.Register(staticProvider{id: "kubeconfig"})

	svc := NewCredentialService(store, reg)
	err := svc.Renew(context.Background(), "kube-1")
	if err == nil {
		t.Fatal("expected renew to be refused for a non-renewable provider")
	}
}

func TestCredential_RenewDelegatesWhenSupported(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{Credentials: []domain.Credential{
		{ID: "az-1", ProviderID: "azure"},
	}}
	reg := providers.NewRegistry()
	_ = reg.Register(fakeProvider{id: "azure"}) // CanRenew: true, Renew returns nil

	svc := NewCredentialService(store, reg)
	if err := svc.Renew(context.Background(), "az-1"); err != nil {
		t.Fatalf("expected renew to succeed, got %v", err)
	}
}
