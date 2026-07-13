package domain

import "time"

// AccessSource is a concrete source from which access information is
// discovered: an Azure CLI profile, an AWS profile/config entry, or later a
// kubeconfig file. It is distinct from Provider (the abstract backend) so we
// can tell "the azure provider" apart from "this particular az login state".
type AccessSource struct {
	ID         SourceID   `json:"id"`
	ProviderID ProviderID `json:"provider_id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Location   string     `json:"location,omitempty"`
	LastSeenAt time.Time  `json:"last_seen_at"`

	Metadata map[string]string `json:"metadata,omitempty"`
}
