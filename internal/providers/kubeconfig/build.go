package kubeconfig

import (
	"strconv"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// ID derivation. There is a single logical source (the kubeconfig); scopes are
// per cluster, credentials per user, and targets per context. Context names are
// unique within a kubeconfig, which makes them stable target IDs.
func sourceID() domain.SourceID             { return "kubeconfig:source" }
func scopeID(cluster string) domain.ScopeID { return domain.ScopeID("kubeconfig:cluster:" + cluster) }
func credentialID(user string) domain.CredentialID {
	return domain.CredentialID("kubeconfig:user:" + user)
}
func targetID(context string) domain.TargetID {
	return domain.TargetID("kubeconfig:context:" + context)
}

// buildSource models the active kubeconfig as the single access source.
func buildSource(location string, now time.Time) domain.AccessSource {
	return domain.AccessSource{
		ID:         sourceID(),
		ProviderID: ProviderID,
		Name:       "kubeconfig",
		Kind:       "kubeconfig",
		Location:   location,
		LastSeenAt: now,
		Metadata:   map[string]string{"location": location},
	}
}

// buildScope models a kubeconfig cluster entry as a scope.
func buildScope(c kcNamedCluster) domain.Scope {
	return domain.Scope{
		ID:         scopeID(c.Name),
		ProviderID: ProviderID,
		SourceID:   sourceID(),
		Name:       c.Name,
		Kind:       "cluster",
		Metadata:   map[string]string{"server": c.Cluster.Server},
	}
}

// buildCredential models a kubeconfig user entry as a credential.
func buildCredential(user, authType string, health domain.AccessHealth, action domain.ActionHint, now time.Time) domain.Credential {
	return domain.Credential{
		ID:         credentialID(user),
		ProviderID: ProviderID,
		SourceID:   sourceID(),
		Name:       user,
		Identity:   user,
		Health:     health,
		ActionHint: action,
		LastSeenAt: now,
		Metadata:   map[string]string{"user": user, "auth_type": authType},
	}
}

// buildTarget maps a kubeconfig context to a target, inheriting its user's
// health. Like the cloud providers it sets only SystemLabels; UserLabels are
// re-attached later by the discovery service.
func buildTarget(cx kcNamedContext, server string, health domain.AccessHealth, action domain.ActionHint, current bool, now time.Time) domain.Target {
	sys := map[string]string{
		domain.LabelProvider: string(ProviderID),
		domain.LabelSource:   string(sourceID()),
		domain.LabelPlatform: "kubeconfig",
		domain.LabelHealth:   string(health),
	}
	return domain.Target{
		ID:           targetID(cx.Name),
		ProviderID:   ProviderID,
		SourceID:     sourceID(),
		CredentialID: credentialID(cx.Context.User),
		ScopeID:      scopeID(cx.Context.Cluster),
		Kind:         "context",
		Name:         cx.Name,
		Endpoint:     server,
		Platform:     "kubeconfig",
		Health:       health,
		ActionHint:   action,
		LastSeenAt:   now,
		// A kubeconfig has no server version to read (its data source is a
		// static file, not a live cluster), so the version is unknowable here.
		KubernetesVersion: domain.VersionUnknown,
		SystemLabels:      sys,
		Metadata: map[string]string{
			"cluster":   cx.Context.Cluster,
			"user":      cx.Context.User,
			"namespace": cx.Context.Namespace,
			"current":   strconv.FormatBool(current),
		},
	}
}
