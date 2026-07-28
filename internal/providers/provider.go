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

// ContextActivator is an optional capability: providers that can materialize a
// target into the local kubeconfig implement it (and report CanSwitchContext).
// It is separate from Provider so adding it never forces every provider (or
// test stub) to implement kubeconfig handling. Services reach it with a type
// assertion, gated on the CanSwitchContext capability.
type ContextActivator interface {
	// Activate fetches the target's credentials into the user's kubeconfig and
	// makes it the current context (e.g. `az aks get-credentials`,
	// `aws eks update-kubeconfig`).
	Activate(ctx context.Context, target domain.Target) error
}

// CredentialActivator is an optional refinement of ContextActivator for
// providers where several credentials can reach the same target (AWS profiles
// sharing an account), so the operator can choose which one to go in through.
//
// It is a separate interface rather than an extra parameter on Activate for two
// reasons. Adding the parameter would force azure, gcp and kubeconfig to accept
// a concept they have no use for — one target, one way in. And the alternative
// of letting the service rewrite provider-specific metadata on the target to
// steer activation would put knowledge of an adapter's internals in the
// provider-agnostic core, which AGENTS.md forbids.
//
// Services reach it with a type assertion, like ContextActivator. A provider
// that does not implement it can only be activated through the target's
// primary credential, and callers must reject an explicit choice rather than
// silently activating something else.
type CredentialActivator interface {
	// ActivateAs fetches the target's credentials into the user's kubeconfig
	// using the given credential rather than the target's recorded primary.
	ActivateAs(ctx context.Context, target domain.Target, cred domain.Credential) error
}

// AccessChecker is an optional capability: providers that can tell which
// credentials a target admits *from inside* — as opposed to which can
// authenticate to the provider about it — implement it. For AWS that is the EKS
// access-entry layer; azure, gcp and kubeconfig have no equivalent this tool
// reads, so they do not implement it and services reach it by type assertion.
//
// It takes every credential that reaches the target and returns the admitted
// subset, rather than answering about one. Both cost a single API call — the
// entry list names all principals — but only this shape lets a caller render a
// verdict per profile without looping.
type AccessChecker interface {
	CheckAccess(ctx context.Context, target domain.Target, creds []domain.Credential) (AccessCheck, error)
}

// AccessCheck is what one live lookup established about a target.
//
// It carries facts, not a verdict: which credentials are listed, and under which
// mode they were read. Callers derive the verdict with domain.AccessVerdictFor,
// so the rule that only `api` mode may produce a negative lives in exactly one
// place.
type AccessCheck struct {
	Mode     domain.AccessCheckMode
	Operable []domain.CredentialID
	// Reason explains why nothing conclusive came back, phrased for display and
	// empty when Mode is conclusive. A format regression must not be worded like
	// a routine "nothing to tell": the two are indistinguishable to a reader
	// otherwise, and one of them is a bug worth chasing.
	Reason string
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
