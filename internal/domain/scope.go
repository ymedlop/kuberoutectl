package domain

// Scope is an administrative or logical boundary that sits *before* the
// Kubernetes cluster: an Azure subscription, an AWS account/profile/role, a
// future GCP project.
//
// Scope is intentionally a separate type from Target. Some providers have a
// real hierarchy (subscription -> AKS, account -> EKS) and routing, re-auth,
// and health all key off that boundary. Collapsing Scope into Target because
// one provider looks flat would break the model the moment a second cluster
// shares a subscription.
type Scope struct {
	ID         ScopeID    `json:"id"`
	ProviderID ProviderID `json:"provider_id"`
	SourceID   SourceID   `json:"source_id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`

	Metadata map[string]string `json:"metadata,omitempty"`
}
