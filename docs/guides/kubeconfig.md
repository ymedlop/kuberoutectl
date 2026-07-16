---
title: kubeconfig
parent: Provider guides
nav_order: 4
---

# kubeconfig guide

How to use `kuberoutectl` to bring the clusters already in your **kubeconfig**
— self-hosted, on-prem, homelab, kind/minikube, or clusters someone handed you a
context for — into the same inventory as your cloud clusters, and switch between
them by short alias. See the [shared model](index.md) for the concepts.

## What makes kubeconfig different

Azure and AWS *authenticate* you; a kubeconfig is just a **static file** that
already contains everything. So this provider doesn't log in or fetch anything —
it reads what's there and lets you organize and switch it:

| kubeconfig element | maps to        | notes                                  |
|--------------------|----------------|----------------------------------------|
| `clusters[]`       | **Scope**      | kind `cluster`, carries the API server |
| `users[]`          | **Credential** | health from how it authenticates       |
| `contexts[]`       | **Target**     | what you actually select and use       |

Consequently: **nothing here is renewable by kuberoutectl** (`CanRenew` is
false). A client certificate or bearer token is `static`; an `exec` / auth-provider
credential (the aws/gcp/oidc plugins) is `unknown`, because it's refreshed on
demand by its plugin, outside our view. Neither is a failure — it's just honest.

## Prerequisites

- **`kubectl`** installed and on your `PATH`. `kuberoutectl` resolves it
  (config path → managed runtime → `PATH` → error) and reads your kubeconfig
  through it.
- A kubeconfig at `~/.kube/config`, or the paths in `$KUBECONFIG` (both merged,
  exactly as `kubectl` sees them).

```console
$ kuberoutectl doctor
CHECK                STATUS  DETAIL
provider:kubeconfig  ok      resolved at /usr/local/bin/kubectl
```

## 1. Discover your contexts

```console
$ kuberoutectl sync kubeconfig
Syncing kubeconfig ...
  → reading kubeconfig (kubectl config view)
  → found 2 cluster(s), 2 user(s), 2 context(s)
  → discovered 2 context(s)
Synced provider: kubeconfig
  sources:     1
  credentials: 2
  scopes:      2
  targets:     2
```

This runs `kubectl config view --raw -o json` and maps it in. No credentials are
read out of the file — only *how* each user authenticates, to classify health.

```console
$ kuberoutectl target list --provider kubeconfig
ALIAS     PLATFORM    REGION  HEALTH   PROVIDER
homelab   kubeconfig          static   kubeconfig
prod-eks  kubeconfig          unknown  kubeconfig
```

The **ALIAS** is the context name (kubeconfig names are already short), usable
directly with `target use`/`inspect`/`label`. `REGION` is blank — kubeconfig has
no region concept.

## 2. Check credential health

```console
$ kuberoutectl credential list --provider kubeconfig
ID                             PROVIDER    IDENTITY       HEALTH   ACTION
kubeconfig:user:prod-eks-user  kubeconfig  prod-eks-user  unknown  none
kubeconfig:user:homelab-admin  kubeconfig  homelab-admin  static   none
```

- `static` — a client certificate / token / basic auth. Long-lived, nothing to
  renew here. (If it expires, fix it at the source, e.g. re-issue the cert.)
- `unknown` — an `exec` or auth-provider user; the plugin manages refresh.

There is no `renew` action for kubeconfig — the provider reports `CanRenew:false`.

## 3. Switch to a cluster

```console
$ kuberoutectl target use homelab
Now using target: homelab (homelab)
kubeconfig updated and set as the current context.
```

Because the context already exists, this is just `kubectl config use-context
<name>` — no credential fetch. Verify:

```bash
kubectl config current-context   # -> homelab
kubectl get nodes
```

`--no-kubeconfig` records the selection without switching your current context.

## 4. Organize alongside the cloud

kubeconfig targets carry the same system labels and take user labels like any
other, so they mix into cross-provider collections:

```bash
kuberoutectl target label add homelab env=lab
kuberoutectl target label add prod-eks env=prod
kuberoutectl collection create prod --selector env=prod   # can span aks/eks/kubeconfig
```

## Duplicate clusters across providers

Running a cloud CLI (e.g. `aws eks update-kubeconfig`) writes a context into your
kubeconfig, so the *same* cluster can be discovered twice: once natively by the
cloud provider and once here. `kuberoutectl` detects this by the **API-server
endpoint** — identical across both — and keeps only the native target, which
carries the richer region and renewable-credential information. The kubeconfig
duplicate is suppressed, and `sync kubeconfig` reports how many:

```console
$ kuberoutectl sync kubeconfig
Syncing kubeconfig ...
  → reading kubeconfig (kubectl config view)
  → suppressed 1 overlay context(s) already discovered natively
  ...
```

This is endpoint-based, not name-based, so it never guesses: a self-hosted
context with its own endpoint (like `homelab`) always survives. The order you
sync in doesn't matter — the native target wins either way.

## Capability summary (kubeconfig)

| Capability          | kubeconfig | Notes                                          |
|---------------------|------------|------------------------------------------------|
| Discover scopes     | yes        | clusters via `kubectl config view`             |
| Credential renew    | no         | static / externally-managed; nothing to renew  |
| Switch context      | yes        | `kubectl config use-context`                    |
| Static credentials  | yes        | certs/tokens reported `static`, exec `unknown` |

## Troubleshooting

- **`sync kubeconfig` finds nothing** — you have no contexts; check
  `kubectl config get-contexts`. An empty kubeconfig is not an error.
- **A context shows `unknown`** — expected for `exec`/auth-provider users; it
  still works via `target use` (kubectl runs the plugin).
- **A context is missing after syncing a cloud provider** — expected if it
  duplicates a natively-discovered cluster (same API-server endpoint); the native
  target wins. See [Duplicate clusters across providers](#duplicate-clusters-across-providers).
- **`kubectl` not found** — install it or set an explicit path in config;
  `kuberoutectl doctor` shows what it resolved.
- **Wrong `$KUBECONFIG`** — `kuberoutectl` sees exactly what `kubectl` sees;
  confirm with `kubectl config view --minify`.
