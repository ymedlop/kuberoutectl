package providers

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Registry holds registered providers. It is safe for concurrent use. There
// is no plugin loading or reflection here — providers are registered
// explicitly at startup, which keeps wiring visible and deterministic.
type Registry struct {
	mu        sync.RWMutex
	providers map[domain.ProviderID]Provider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[domain.ProviderID]Provider)}
}

// Register adds a provider. It errors on a nil provider, an empty ID, or a
// duplicate ID rather than silently overwriting — a double registration is a
// wiring bug we want surfaced.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return fmt.Errorf("cannot register nil provider")
	}
	id := p.ID()
	if id == "" {
		return fmt.Errorf("cannot register provider with empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider %q already registered", id)
	}
	r.providers[id] = p
	return nil
}

// Get returns the provider for id, or false if none is registered.
func (r *Registry) Get(id domain.ProviderID) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// List returns all registered providers sorted by ID for deterministic output.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
