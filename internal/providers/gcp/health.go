package gcp

import "github.com/ymedlop/kuberoutectl/internal/domain"

// activeAccount resolves the effective gcloud identity. The `gcloud auth list`
// ACTIVE entry is authoritative; if none is marked active (older gcloud, or a
// config-only account) it falls back to the account from `gcloud config list`.
// It returns ok=false when there is no usable account at all — the logged-out
// case.
func activeAccount(configAccount string, accounts []gcpAuthAccount) (string, bool) {
	for _, a := range accounts {
		if a.Status == "ACTIVE" && a.Account != "" {
			return a.Account, true
		}
	}
	if configAccount != "" {
		return configAccount, true
	}
	return "", false
}

// mapGCPHealth turns "is there an active account" into health and next action.
// GCP credentials are OAuth-backed and renewable, so — like Azure — the model
// is binary here: authed maps to valid/use, logged out maps to expired/renew
// (`gcloud auth login`). Fine-grained token-expiry is a later refinement.
func mapGCPHealth(authed bool) (domain.AccessHealth, domain.ActionHint) {
	if authed {
		return domain.HealthValid, domain.ActionUse
	}
	return domain.HealthExpired, domain.ActionRenew
}
