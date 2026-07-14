# GCP

Use the gcloud CLI as the source of truth for GCP identity, projects, and GKE.

## Shape

Per-login, like Azure: one active gcloud account spans many projects. Single
AccessSource + Credential; projects → Scopes (kind `project`); GKE clusters →
Targets (kind `gke`). See `internal/providers/gcp/`.

## Commands used

- `gcloud config list --format=json` — active account/project
- `gcloud auth list --format=json` — the ACTIVE entry is authoritative;
  fall back to the config account if none is marked active
- `gcloud projects list --format=json`
- `gcloud container clusters list --project <id> --format=json`
- `gcloud container clusters get-credentials <name> --location <loc> --project <id>` (Activate)
- `gcloud auth login [account]` (Renew — interactive browser/device flow)

## Notes

- GKE `location` is a region (`europe-west1`) or a zone (`europe-west4-a`);
  `--location` accepts either. It maps to Target.Region.
- A project without the GKE (Container) API enabled fails `clusters list` —
  skip it, don't fail the sync. A top-level `projects list` failure while
  authenticated is rarer and more actionable (IAM permission): emit a
  `prog.Step` diagnostic.
- Endpoint from the API is a bare IP; prefix `https://`.
- Health is binary for now (active account → valid, none → expired/renew);
  token-expiry granularity and service-account keys are tracked in TODO.md.
