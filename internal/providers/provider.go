// Package providers defines the provider contract and a compile-time
// registry. Shared services depend on the Provider interface here, never on
// a concrete provider package (azure, aws, ...). New providers plug in by
// implementing Provider and registering themselves — no core changes needed.
package providers

import (
	"context"
	"errors"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// ErrUnsupported is returned by capability-gated operations a provider does
// not implement (e.g. Renew on a static kubeconfig provider).
var ErrUnsupported = errors.New("operation not supported by provider")

// DiscoveryInput carries whatever a discovery run needs from the caller.
// Kept minimal for the spine; providers extend behavior via their own config.
type DiscoveryInput struct {
	// Progress receives human-readable status updates as discovery proceeds, so
	// the CLI can show the user that a slow sync (e.g. many subscriptions, or an
	// az/aws call waiting on auth) is actually working. May be nil.
	Progress Progress
}

// Progress receives step updates during discovery. Providers call Step at
// meaningful points; the CLI renders them. It is deliberately tiny so any
// caller can supply one.
type Progress interface {
	Step(format string, args ...any)
}

// NopProgress is a Progress that discards updates, for callers (and tests)
// that don't want output.
type NopProgress struct{}

// Step implements Progress.
func (NopProgress) Step(string, ...any) {}

// ProgressOr returns p, or a NopProgress if p is nil, so provider code can call
// Step unconditionally regardless of whether the caller supplied one.
func ProgressOr(p Progress) Progress {
	if p == nil {
		return NopProgress{}
	}
	return p
}

// DiscoveryResult is what a provider returns from a discovery run. Targets
// carry SystemLabels only; UserLabels are re-attached later by the discovery
// service, so a provider must never populate them.
type DiscoveryResult struct {
	Sources     []domain.AccessSource
	Credentials []domain.Credential
	Scopes      []domain.Scope
	Targets     []domain.Target
}

// Provider is the full contract a backend implements. It is small on purpose:
// discover state, and optionally renew a credential. Everything else
// (organization, persistence, selection) is core concern, not provider concern.
type Provider interface {
	ID() domain.ProviderID
	Capabilities() domain.Capabilities

	// Discover reads current access state from the provider's sources.
	Discover(ctx context.Context, in DiscoveryInput) (DiscoveryResult, error)

	// Renew refreshes or re-authenticates a credential. Providers whose
	// Capabilities report CanRenew=false should return ErrUnsupported.
	Renew(ctx context.Context, cred domain.Credential) error
}
