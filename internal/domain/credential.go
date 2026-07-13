package domain

import "time"

// Credential is a usable identity inside a provider. It carries health and
// an action hint because "what state is this identity in" and "what should I
// do about it" are the two questions the operator most often asks.
//
// ExpiresAt is optional: static credentials (e.g. a kubeconfig client cert)
// legitimately have no expiry, in which case Health is HealthStatic and
// ExpiresAt is nil rather than a zero time pretending to be meaningful.
type Credential struct {
	ID         CredentialID `json:"id"`
	ProviderID ProviderID   `json:"provider_id"`
	SourceID   SourceID     `json:"source_id"`
	Name       string       `json:"name"`
	Identity   string       `json:"identity,omitempty"`
	Health     AccessHealth `json:"health"`
	ActionHint ActionHint   `json:"action_hint"`
	ExpiresAt  *time.Time   `json:"expires_at,omitempty"`
	LastSeenAt time.Time    `json:"last_seen_at"`

	Metadata map[string]string `json:"metadata,omitempty"`
}
