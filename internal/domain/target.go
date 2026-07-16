package domain

import (
	"strconv"
	"time"
)

// Target is a selectable Kubernetes destination: an AKS cluster, an EKS
// cluster, later a GKE cluster or a kubeconfig context. Targets are where
// user organization becomes most valuable, so they carry two disjoint label
// sets.
//
//   - SystemLabels are tool-owned (kuberoutectl.io/* namespace). Discovery
//     writes them; the user cannot.
//   - UserLabels are user-owned. Discovery must NEVER write them. They are
//     persisted separately (state/user-labels.json) and re-attached to freshly
//     discovered targets by ID, which is what lets them survive a resync.
//
// Keeping the maps separate on the type — not merged into one — is the
// in-memory half of that guarantee.
type Target struct {
	ID           TargetID     `json:"id"`
	ProviderID   ProviderID   `json:"provider_id"`
	SourceID     SourceID     `json:"source_id"`
	CredentialID CredentialID `json:"credential_id"`
	ScopeID      ScopeID      `json:"scope_id"`
	Kind         string       `json:"kind"`
	Name         string       `json:"name"`
	// Alias is a short, stable, human-friendly handle for the target, usable
	// anywhere the full ID is (use/inspect/label). It is derived from the name
	// and made unique across the fleet, so it is a presentation/service concern
	// rather than provider-owned identity — providers leave it empty and the
	// service layer fills it in on read.
	Alias      string       `json:"alias,omitempty"`
	Endpoint   string       `json:"endpoint,omitempty"`
	Region     string       `json:"region,omitempty"`
	Platform   string       `json:"platform,omitempty"`
	Health     AccessHealth `json:"health"`
	ActionHint ActionHint   `json:"action_hint"`
	LastSeenAt time.Time    `json:"last_seen_at"`

	// Hidden is a computed-on-read flag (like Alias) — never stored in the
	// snapshot. It is the read-time join between a target and the user-owned
	// hidden-ID set, so hiding survives a resync. Populated wherever the selector
	// engine runs or a target is surfaced; providers leave it false.
	Hidden bool `json:"hidden,omitempty"`

	SystemLabels map[string]string `json:"system_labels,omitempty"`
	UserLabels   map[string]string `json:"user_labels,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
}

// EffectiveLabels returns a single map with user labels taking precedence
// over system labels. It never mutates the target.
func (t Target) EffectiveLabels() map[string]string {
	out := make(map[string]string, len(t.SystemLabels)+len(t.UserLabels))
	for k, v := range t.SystemLabels {
		out[k] = v
	}
	for k, v := range t.UserLabels {
		out[k] = v
	}
	return out
}

// SelectionLabels is the key/value space a selector evaluates against. On top
// of the real labels it exposes a target's structured attributes under bare,
// ergonomic keys (region, platform, provider, health, kind) so operators can
// write `region in [westeurope]` — matching the README — without spelling out
// the kuberoutectl.io/ namespace. Precedence, lowest to highest: structured
// aliases, then system labels, then user labels (a user label always wins).
func (t Target) SelectionLabels() map[string]string {
	out := map[string]string{}
	setNonEmpty := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	setNonEmpty("region", t.Region)
	setNonEmpty("platform", t.Platform)
	setNonEmpty("provider", string(t.ProviderID))
	setNonEmpty("kind", t.Kind)
	setNonEmpty("health", string(t.Health))
	// Visibility is always exposed (both keys, both states) so selectors can
	// filter on it and the default-hide rule can detect a visibility constraint.
	// User labels can't shadow these — ValidateUserLabelKey reserves the keys.
	out["visible"] = strconv.FormatBool(!t.Hidden)
	out["hidden"] = strconv.FormatBool(t.Hidden)
	for k, v := range t.SystemLabels {
		out[k] = v
	}
	for k, v := range t.UserLabels {
		out[k] = v
	}
	return out
}
