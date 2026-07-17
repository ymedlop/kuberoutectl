package aws

import (
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

// buildTarget maps an EKS cluster to a target. Like Azure it sets only
// SystemLabels; UserLabels are re-attached later by the discovery service.
func buildTarget(profile, region string, id awsIdentity, c awsCluster, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Target {
	sys := map[string]string{
		domain.LabelProvider: string(ProviderID),
		domain.LabelSource:   string(sourceID(profile)),
		domain.LabelPlatform: "eks",
		domain.LabelHealth:   string(health),
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
