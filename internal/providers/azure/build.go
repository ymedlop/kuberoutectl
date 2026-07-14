package azure

import (
	"sort"
	"strconv"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// credentialID derives a stable credential identity from an account. It keys
// on tenant + user so the same login maps to one credential across its
// subscriptions, while distinct tenant logins stay distinct.
func credentialID(a azAccount) domain.CredentialID {
	return domain.CredentialID("azure:" + a.TenantID + ":" + a.User.Name)
}

// buildSources returns the single AccessSource for the az CLI login state.
func buildSources(now time.Time) []domain.AccessSource {
	return []domain.AccessSource{{
		ID:         sourceID,
		ProviderID: ProviderID,
		Name:       "azure-cli",
		Kind:       "cli",
		LastSeenAt: now,
	}}
}

// buildCredentials produces one credential per distinct login identity.
//
// Health/action are account-level here: Azure keeps a single token cache, so a
// single probe reasonably describes the common single-tenant login. For
// multi-tenant logins this is an accepted MVP approximation — a later slice can
// probe per tenant if needed.
func buildCredentials(accounts []azAccount, health domain.AccessHealth, action domain.ActionHint, now time.Time) []domain.Credential {
	seen := map[domain.CredentialID]bool{}
	var out []domain.Credential
	for _, a := range accounts {
		id := credentialID(a)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, domain.Credential{
			ID:         id,
			ProviderID: ProviderID,
			SourceID:   sourceID,
			Name:       a.User.Name,
			Identity:   a.User.Name,
			Health:     health,
			ActionHint: action,
			LastSeenAt: now,
			Metadata: map[string]string{
				"tenant_id": a.TenantID,
				"user_type": a.User.Type,
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// buildScopes maps subscriptions to scopes. Subscription is Azure's primary
// scope abstraction — kept distinct from targets so a subscription with
// multiple AKS clusters models correctly.
func buildScopes(accounts []azAccount) []domain.Scope {
	out := make([]domain.Scope, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, domain.Scope{
			ID:         domain.ScopeID(a.ID),
			ProviderID: ProviderID,
			SourceID:   sourceID,
			Name:       a.Name,
			Kind:       "subscription",
			Metadata: map[string]string{
				"tenant_id":  a.TenantID,
				"state":      a.State,
				"is_default": strconv.FormatBool(a.IsDefault),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// buildTargets maps AKS clusters in one subscription to targets. It sets only
// SystemLabels (kuberoutectl.io/*); UserLabels are left empty for the
// discovery service to re-attach from user state.
func buildTargets(acc azAccount, clusters []azCluster, health domain.AccessHealth, action domain.ActionHint, now time.Time) []domain.Target {
	out := make([]domain.Target, 0, len(clusters))
	for _, c := range clusters {
		endpoint := ""
		if c.Fqdn != "" {
			endpoint = "https://" + c.Fqdn
		}
		sys := map[string]string{
			domain.LabelProvider: string(ProviderID),
			domain.LabelSource:   string(sourceID),
			domain.LabelPlatform: "aks",
			domain.LabelHealth:   string(health),
		}
		if c.Location != "" {
			sys[domain.LabelRegion] = c.Location
		}
		out = append(out, domain.Target{
			ID:           domain.TargetID(c.ID),
			ProviderID:   ProviderID,
			SourceID:     sourceID,
			CredentialID: credentialID(acc),
			ScopeID:      domain.ScopeID(acc.ID),
			Kind:         "aks",
			Name:         c.Name,
			Endpoint:     endpoint,
			Region:       c.Location,
			Platform:     "aks",
			Health:       health,
			ActionHint:   action,
			LastSeenAt:   now,
			SystemLabels: sys,
			Metadata: map[string]string{
				"resource_group":     c.ResourceGroup,
				"kubernetes_version": c.KubernetesVersion,
				"power_state":        c.PowerState.Code,
				"provisioning_state": c.ProvisioningState,
			},
		})
	}
	return out
}

// notLoggedInResult represents an az session that is absent or unusable. It
// surfaces a single expired credential hinting renewal, so `credential list`
// and `doctor` can tell the operator to log in rather than showing nothing.
func notLoggedInResult(now time.Time, detail string) providers.DiscoveryResult {
	meta := map[string]string{}
	if detail != "" {
		meta["detail"] = detail
	}
	return providers.DiscoveryResult{
		Sources: buildSources(now),
		Credentials: []domain.Credential{{
			ID:         "azure:not-logged-in",
			ProviderID: ProviderID,
			SourceID:   sourceID,
			Name:       "azure-cli",
			Health:     domain.HealthExpired,
			ActionHint: domain.ActionRenew,
			LastSeenAt: now,
			Metadata:   meta,
		}},
	}
}
