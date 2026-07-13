package providers

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// stubProvider is a minimal Provider for registry tests.
type stubProvider struct {
	id   domain.ProviderID
	caps domain.Capabilities
}

func (s stubProvider) ID() domain.ProviderID             { return s.id }
func (s stubProvider) Capabilities() domain.Capabilities { return s.caps }
func (s stubProvider) Discover(context.Context, DiscoveryInput) (DiscoveryResult, error) {
	return DiscoveryResult{}, nil
}
func (s stubProvider) Renew(context.Context, domain.Credential) error { return ErrUnsupported }

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := stubProvider{id: "azure", caps: domain.Capabilities{CanRenew: true}}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("azure")
	if !ok {
		t.Fatal("Get(azure): not found after register")
	}
	if got.ID() != "azure" || !got.Capabilities().CanRenew {
		t.Fatalf("unexpected provider back: %+v", got.Capabilities())
	}
}

func TestRegistry_RejectsNilEmptyAndDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("expected error registering nil provider")
	}
	if err := r.Register(stubProvider{id: ""}); err == nil {
		t.Error("expected error registering empty-ID provider")
	}
	if err := r.Register(stubProvider{id: "aws"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(stubProvider{id: "aws"}); err == nil {
		t.Error("expected error registering duplicate ID")
	}
}

func TestRegistry_ListSortedByID(t *testing.T) {
	r := NewRegistry()
	for _, id := range []domain.ProviderID{"kubeconfig", "azure", "gcp", "aws"} {
		if err := r.Register(stubProvider{id: id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	list := r.List()
	want := []domain.ProviderID{"aws", "azure", "gcp", "kubeconfig"}
	if len(list) != len(want) {
		t.Fatalf("len = %d, want %d", len(list), len(want))
	}
	for i, p := range list {
		if p.ID() != want[i] {
			t.Errorf("List()[%d] = %s, want %s", i, p.ID(), want[i])
		}
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nope"); ok {
		t.Error("Get on empty registry should return ok=false")
	}
}
