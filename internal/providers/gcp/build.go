package gcp

import (
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// ID derivation. There is a single logical source and credential (the active
// gcloud login, Azure-style); scopes are per project and targets per GKE
// cluster. A cluster is uniquely identified by project + location + name.
func sourceID() domain.SourceID { return "gcp:source" }
func credentialID(account string) domain.CredentialID {
	if account == "" {
		return "gcp:credential"
	}
	return domain.CredentialID("gcp:account:" + account)
}
func scopeID(projectID string) domain.ScopeID { return domain.ScopeID("gcp:project:" + projectID) }
func targetID(projectID, location, name string) domain.TargetID {
	return domain.TargetID("gcp:" + projectID + ":" + location + ":" + name)
}

// buildSource models the active gcloud configuration as the single source.
func buildSource(account string, now time.Time) domain.AccessSource {
	return domain.AccessSource{
		ID:         sourceID(),
		ProviderID: ProviderID,
		Name:       "gcloud",
		Kind:       "gcloud",
		Location:   "~/.config/gcloud",
		LastSeenAt: now,
		Metadata:   map[string]string{"account": account},
	}
}

// buildCredential models the active gcloud account as the credential.
func buildCredential(account, project string, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Credential {
	return domain.Credential{
		ID:         credentialID(account),
		ProviderID: ProviderID,
		SourceID:   sourceID(),
		Name:       account,
		Identity:   account,
		Health:     health,
		ActionHint: action,
		LastSeenAt: now,
		Metadata:   map[string]string{"account": account, "project": project},
	}
}

// buildScope models a GCP project as a scope.
func buildScope(p gcpProject) domain.Scope {
	name := p.Name
	if name == "" {
		name = p.ProjectID
	}
	return domain.Scope{
		ID:         scopeID(p.ProjectID),
		ProviderID: ProviderID,
		SourceID:   sourceID(),
		Name:       name,
		Kind:       "project",
		Metadata:   map[string]string{"project_id": p.ProjectID, "project_number": p.ProjectNumber},
	}
}

// buildTarget maps a GKE cluster to a target. Like the other providers it sets
// only SystemLabels; UserLabels are re-attached later by the discovery service.
func buildTarget(account string, p gcpProject, c gcpCluster, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Target {
	sys := map[string]string{
		domain.LabelProvider: string(ProviderID),
		domain.LabelSource:   string(sourceID()),
		domain.LabelPlatform: "gke",
		domain.LabelHealth:   string(health),
	}
	if c.Location != "" {
		sys[domain.LabelRegion] = c.Location
	}
	endpoint := c.Endpoint
	if endpoint != "" {
		endpoint = "https://" + endpoint
	}
	return domain.Target{
		ID:                targetID(p.ProjectID, c.Location, c.Name),
		ProviderID:        ProviderID,
		SourceID:          sourceID(),
		CredentialID:      credentialID(account),
		ScopeID:           scopeID(p.ProjectID),
		Kind:              "gke",
		Name:              c.Name,
		Endpoint:          endpoint,
		Region:            c.Location,
		Platform:          "gke",
		Health:            health,
		ActionHint:        action,
		LastSeenAt:        now,
		KubernetesVersion: domain.NormalizeKubernetesVersion(c.CurrentMasterVersion),
		SystemLabels:      sys,
		Metadata: map[string]string{
			"project":  p.ProjectID,
			"location": c.Location,
			"status":   c.Status,
		},
	}
}
