// Package domain holds the provider-agnostic core types for kuberoutectl.
//
// It has no knowledge of Cobra, of external CLIs, or of how state is
// persisted. Every other package depends on domain; domain depends on
// nothing in this repository. Keep it that way.
package domain

// Strongly typed identifiers. They are plain strings underneath, but the
// distinct types stop us from accidentally passing a ScopeID where a
// TargetID is expected — a real hazard given how many IDs flow through
// discovery and selection.
type (
	ProviderID   string
	SourceID     string
	CredentialID string
	ScopeID      string
	TargetID     string
	CollectionID string
)
