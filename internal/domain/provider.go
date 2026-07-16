package domain

// Capabilities declares what a provider supports. The core reads these
// instead of assuming every provider behaves like Azure — this is what keeps
// provider-specific conditionals out of the shared services.
//
// The split matters: Capabilities gate the *menu* of actions a provider can
// offer, while per-Credential Health/ActionHint decide the actual item. AWS,
// for example, reports CanRenew=true as a provider, but a static-key profile
// still resolves to ActionManual per credential.
type Capabilities struct {
	CanDiscoverScopes bool `json:"can_discover_scopes"`
	CanRenew          bool `json:"can_renew"`
	CanReauth         bool `json:"can_reauth"`
	CanSwitchContext  bool `json:"can_switch_context"`
	StaticCredentials bool `json:"static_credentials"`

	// OverlayProvider marks a provider whose targets are an overlay view of
	// clusters that another provider may own natively (e.g. kubeconfig contexts
	// written by `aws eks update-kubeconfig`). During a sync, an overlay
	// target is suppressed when a non-overlay target shares its endpoint, so the
	// richer native target wins. Cross-provider concern; the core reads this
	// flag rather than special-casing a provider by name.
	OverlayProvider bool `json:"overlay_provider"`
}
