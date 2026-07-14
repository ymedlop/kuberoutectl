---
title: GCP (GKE)
parent: Provider guides
nav_order: 3
---

# GCP (GKE) guide

How to use `kuberoutectl` to discover, inspect, and route to **GKE** clusters
across your GCP projects, and keep your gcloud login healthy. See the
[shared model](index.md) for the concepts and the credential-health spectrum.

## How GCP maps in

GCP looks like Azure more than AWS: **one active login spans many projects**
(as an Azure login spans subscriptions). So there is a single source and a
single credential — your active gcloud account — with projects as scopes and
GKE clusters as targets.

| GCP element                       | domain type    | notes                          |
|-----------------------------------|----------------|--------------------------------|
| active gcloud config/account      | **AccessSource** + **Credential** | one login |
| `gcloud projects list`            | **Scope**      | kind `project`                 |
| `gcloud container clusters list`  | **Target**     | kind `gke`, region = location  |

The login is OAuth-backed and **renewable** (`gcloud auth login`), so unlike
kubeconfig this credential participates in the renew lifecycle.

## Prerequisites

- The **Google Cloud CLI** (`gcloud`) installed and on your `PATH`.
  `kuberoutectl` resolves it (config path → managed runtime → `PATH` → error)
  and never bundles it.
- `kubectl` + the GKE auth plugin (`gke-gcloud-auth-plugin`) for actually
  talking to a cluster after `target use`.

```console
$ kuberoutectl doctor
CHECK           STATUS  DETAIL
provider:gcp    ok      resolved at /usr/bin/gcloud
```

## 1. Sign in

```bash
gcloud auth login
gcloud config set project <your-default-project>   # optional
```

Your active account is the **credential**; its health reflects whether gcloud
has an active login.

## 2. Discover your clusters

```console
$ kuberoutectl sync gcp
Syncing gcp ...
  → reading gcloud config and account
  → listing GCP projects (gcloud projects list)
  → found 2 project(s)
  → listing GKE clusters in "platform-prod-123" (1/2)
  → listing GKE clusters in "platform-lab-456" (2/2)
  → discovered 2 cluster(s)
Synced provider: gcp
  sources:     1
  credentials: 1
  scopes:      2
  targets:     2
```

`sync gcp` reads `gcloud config list` / `auth list`, enumerates
`gcloud projects list` (each a Scope), then `gcloud container clusters list`
per project (each cluster a Target). Progress goes to stderr; the summary and
`-o json` go to stdout.

> **Heads-up on project count:** it lists clusters for *every* project you can
> see. For org accounts with many projects that's a lot of API calls — the
> progress lines show which project it's on. Projects without the GKE API
> enabled are skipped, not fatal.

```console
$ kuberoutectl target list --provider gcp
ALIAS          PLATFORM  REGION          HEALTH  PROVIDER
gke-lab-euw4   gke       europe-west4-a  valid   gcp
gke-prod-euw1  gke       europe-west1    valid   gcp
```

`REGION` is the GKE **location** — a region (`europe-west1`) for regional
clusters or a zone (`europe-west4-a`) for zonal ones. The **ALIAS** is the
cluster name, usable with `target use`/`inspect`/`label`.

## 3. Check credential health

```console
$ kuberoutectl credential list --provider gcp
ID                             PROVIDER  IDENTITY           HEALTH  ACTION
gcp:account:yeray@example.com  gcp       yeray@example.com  valid   use
```

- `valid` / `use` — gcloud has an active login.
- `expired` / `renew` — no active account; sign in again.

## 4. Renew when logged out

```console
$ kuberoutectl credential renew gcp:account:yeray@example.com
Renewed credential: gcp:account:yeray@example.com
Run `kuberoutectl sync` to refresh health.
```

This runs `gcloud auth login` (scoped to the recorded account). It's Google's
interactive browser/device flow — `kuberoutectl` doesn't suppress it. Re-run
`kuberoutectl sync gcp` afterward to refresh cached health.

## 5. Route kubectl at a cluster

```console
$ kuberoutectl target use gke-prod-euw1
Now using target: gke-prod-euw1 (gke-prod-euw1)
kubeconfig updated and set as the current context.
```

This runs `gcloud container clusters get-credentials <name> --location
<location> --project <project>`, merging the cluster into `~/.kube/config` and
setting it current. Use `--no-kubeconfig` to record the selection only.

```bash
kubectl config current-context
kubectl get nodes
```

## 6. Organize across projects and clouds

Same model as the other providers — user labels survive resyncs, collections
are live selector views that can span GCP, Azure, AWS, and kubeconfig:

```bash
kuberoutectl target label add gke-prod-euw1 env=prod
kuberoutectl collection create prod --selector env=prod
```

## Capability summary (GCP)

| Capability          | GCP | Notes                                             |
|---------------------|-----|---------------------------------------------------|
| Discover scopes     | yes | projects via `gcloud projects list`               |
| Credential renew    | yes | `gcloud auth login` (interactive)                 |
| Switch context      | yes | `gcloud container clusters get-credentials`       |
| Static credentials  | no  | OAuth login is renewable                          |

## Troubleshooting

- **`sync gcp` shows only a credential, no clusters** — you're logged out
  (`expired`/`renew`) or `gcloud projects list` failed. Run `gcloud auth login`.
- **A project's clusters are missing** — the GKE (Container) API may be disabled
  for that project; enable it or ignore. Discovery skips it without failing.
- **`kubectl` can't authenticate after `target use`** — install the
  `gke-gcloud-auth-plugin` (`gcloud components install gke-gcloud-auth-plugin`).
- **`gcloud` not found** — install the Google Cloud CLI or set an explicit path
  in config; `kuberoutectl doctor` shows what it resolved.

## Known limitations

- Fine-grained token-expiry health isn't modeled yet — the login is reported
  `valid` while an active account exists, `expired` when none does.
- Service-account key files aren't modeled as separate static credentials yet.
