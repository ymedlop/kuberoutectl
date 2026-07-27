package aws

import (
	"sort"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// ID derivation. Sources and credentials are per-profile; scopes are per
// account (a profile operates within one account, and multiple profiles may
// share an account — mirroring Azure's subscription-shared-by-logins shape).
func sourceID(profile string) domain.SourceID         { return domain.SourceID("aws:" + profile) }
func credentialID(profile string) domain.CredentialID { return domain.CredentialID("aws:" + profile) }
func scopeID(account string) domain.ScopeID           { return domain.ScopeID("aws:account:" + account) }

// buildSource models a profile as an access source rooted in the AWS config.
func buildSource(profile string, now time.Time) domain.AccessSource {
	return domain.AccessSource{
		ID:         sourceID(profile),
		ProviderID: ProviderID,
		Name:       profile,
		Kind:       "profile",
		Location:   "~/.aws/config",
		LastSeenAt: now,
		Metadata:   map[string]string{"profile": profile},
	}
}

// buildCredential models a profile's effective identity.
func buildCredential(profile string, id awsIdentity, authType string, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Credential {
	return domain.Credential{
		ID:         credentialID(profile),
		ProviderID: ProviderID,
		SourceID:   sourceID(profile),
		Name:       profile,
		Identity:   id.Arn,
		Health:     health,
		ActionHint: action,
		LastSeenAt: now,
		Metadata: map[string]string{
			"profile":   profile,
			"auth_type": authType,
			"account":   id.Account,
			"user_id":   id.UserID,
		},
	}
}

// buildScope models an AWS account as a scope. Returns ok=false for an empty
// account so callers skip it.
func buildScope(account string) (domain.Scope, bool) {
	if account == "" {
		return domain.Scope{}, false
	}
	return domain.Scope{
		ID:         scopeID(account),
		ProviderID: ProviderID,
		Name:       account,
		Kind:       "account",
		Metadata:   map[string]string{"account": account},
	}, true
}

// credentialRank orders credential health best-first, so a fold can pick the
// profile an operator would actually get in through. It is total: a health the
// domain adds later is unrecognised here and sorts worst, which degrades to
// "never preferred" rather than silently outranking a working credential.
func credentialRank(h domain.AccessHealth) int {
	switch h {
	case domain.HealthValid:
		return 0
	case domain.HealthExpiring:
		return 1
	case domain.HealthStatic:
		return 2
	case domain.HealthExpired:
		return 3
	case domain.HealthError:
		return 4
	case domain.HealthUnknown:
		return 5
	default:
		return 6
	}
}

// foldTargetsByID collapses the per-(profile, cluster) targets discovery
// produces into one target per cluster. Several AWS profiles authenticating
// into the same account see the same cluster, and an EKS ARN is account-scoped:
// without this, discovery emits duplicate targets sharing an ID, which
// AssignAliases then gives the same alias and ResolveTargetRef resolves to the
// first — leaving the rest unreachable by any reference the CLI prints.
//
// The primary is the candidate whose credential ranks best, ties broken by
// profile name so the result never depends on discovery order.
//
// It returns the primary candidate's own struct with CredentialIDs added, and
// deliberately does NOT assemble a target from parts. Every provider-owned
// field — including the SystemLabels and Metadata maps — therefore stays
// consistent with the primary by construction. Patching scalar fields onto some
// other candidate would leave SystemLabels[LabelHealth] describing a different
// profile than Target.Health, and SelectionLabels() exposes both, so a selector
// on `health` and one on `kuberoutectl.io/health` would disagree about the same
// target.
//
// Input order is preserved for the surviving targets; only duplicates collapse.
//
// Maintenance note: the returned target shares its SystemLabels and Metadata
// maps with the input candidate, because copying a Go struct copies map fields
// by reference. Never write into those maps here — reassign the whole map if a
// value must change, or the mutation is also visible through the caller's
// slice.
func foldTargetsByID(targets []domain.Target) []domain.Target {
	groups := map[domain.TargetID][]domain.Target{}
	var order []domain.TargetID
	for _, t := range targets {
		if _, seen := groups[t.ID]; !seen {
			order = append(order, t.ID)
		}
		groups[t.ID] = append(groups[t.ID], t)
	}

	out := make([]domain.Target, 0, len(order))
	for _, id := range order {
		group := groups[id]
		if len(group) == 1 {
			out = append(out, group[0])
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			ri, rj := credentialRank(group[i].Health), credentialRank(group[j].Health)
			if ri != rj {
				return ri < rj
			}
			return group[i].Metadata["profile"] < group[j].Metadata["profile"]
		})
		primary := group[0]
		primary.CredentialIDs = make([]domain.CredentialID, 0, len(group))
		for _, c := range group {
			primary.CredentialIDs = append(primary.CredentialIDs, c.CredentialID)
		}
		out = append(out, primary)
	}
	return out
}

// buildTarget maps an EKS cluster to a target. Like Azure it sets only
// SystemLabels; UserLabels are re-attached later by the discovery service.
func buildTarget(profile, region string, id awsIdentity, c awsCluster, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Target {
	sys := map[string]string{
		domain.LabelProvider:   string(ProviderID),
		domain.LabelSource:     string(sourceID(profile)),
		domain.LabelPlatform:   "eks",
		domain.LabelHealth:     string(health),
		domain.LabelCredential: profile,
	}
	if region != "" {
		sys[domain.LabelRegion] = region
	}
	return domain.Target{
		ID:                domain.TargetID(c.Arn),
		ProviderID:        ProviderID,
		SourceID:          sourceID(profile),
		CredentialID:      credentialID(profile),
		ScopeID:           scopeID(id.Account),
		Kind:              "eks",
		Name:              c.Name,
		Endpoint:          c.Endpoint,
		Region:            region,
		Platform:          "eks",
		Health:            health,
		ActionHint:        action,
		LastSeenAt:        now,
		KubernetesVersion: domain.NormalizeKubernetesVersion(c.Version),
		SystemLabels:      sys,
		Metadata: map[string]string{
			"profile": profile,
			"account": id.Account,
			"status":  c.Status,
		},
	}
}
