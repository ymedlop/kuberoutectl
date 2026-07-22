---
title: Azure (AKS)
description: "Manage AKS clusters and Azure credentials with kuberoutectl — discover subscriptions, check credential health, and route kubectl via the Azure CLI."
parent: Provider guides
nav_order: 1
---

# Azure (AKS) guide

How to use `kuberoutectl` to discover, inspect, and route to **AKS** clusters,
and to keep your Azure login healthy. See the [shared model](index.md) for the
concepts and the credential-health spectrum referenced below.

## Prerequisites

- The **Azure CLI** (`az`) installed and on your `PATH`.
  `kuberoutectl` resolves it in this order: an explicit path in config → a
  managed runtime → `PATH` → a clear error. It never bundles `az`.
- `kubectl` (only needed when you actually run `target use` and then talk to a
  cluster).

Confirm the CLI can be found:

```console
$ kuberoutectl doctor
CHECK        STATUS  DETAIL
azure (az)   ok      /usr/bin/az
aws (aws)    ok      /usr/bin/aws
```

If Azure shows `missing`, install the Azure CLI or point at it via config.

## 1. Sign in (credential management starts here)

`kuberoutectl` uses your existing Azure CLI session — it does not manage a
separate identity. Sign in with the Azure CLI:

```bash
az login
# or, scoped to a directory:
az login --tenant <tenant-id>
```

Your login is the **credential**. Its health is derived from the access-token
expiry Azure reports, so once you sign in, `kuberoutectl` can tell you when the
session is close to expiring or already expired.

## 2. Discover your clusters

```console
$ kuberoutectl sync azure
Syncing azure ...
  → listing subscriptions
  → subscription 1/3: Platform Prod
  → subscription 2/3: Platform Lab
  → subscription 3/3: Sandbox
Synced provider: azure
  sources:     1
  credentials: 1
  scopes:      3
  targets:     3
```

`sync` runs `az account list`, `az account get-access-token`, and
`az aks list` per subscription, then writes the result into the local cache.
Progress lines go to **stderr**; the summary (and `-o json`) go to **stdout**,
so piping stays clean.

> **Nothing happening / looks idle?** `az account get-access-token` blocks when
> your session has expired and Azure needs an interactive re-auth. Run
> `az login` first, then re-run `sync azure`. The progress lines above tell you
> which subscription it is on.

Each subscription becomes a **Scope**; each AKS cluster becomes a **Target**:

```console
$ kuberoutectl inventory scopes
NAME           PROVIDER  ID
Platform Prod  azure     aaaaaaaa-0000-0000-0000-000000000001
Platform Lab   azure     aaaaaaaa-0000-0000-0000-000000000002
Sandbox        azure     aaaaaaaa-0000-0000-0000-000000000003

$ kuberoutectl target list
ALIAS         PLATFORM  REGION      HEALTH  PROVIDER
aks-prod-weu  aks       westeurope  valid   azure
aks-lab-weu   aks       westeurope  valid   azure
```

The **ALIAS** is a short, stable handle (derived from the cluster name) that you
can use anywhere a target is expected — `target use`, `target inspect`,
`target label` — instead of the long ARM resource ID. The full ID is still the
underlying identity (stable across resyncs, what labels/collections attach to);
show it with `--wide` or `-o json`. Filter the list with `--provider azure` or a
selector, e.g. `-l env=prod` or `-l "region in [westeurope, northeurope]"`.
`target inspect` also reports the cluster's **Kubernetes server version**.

## 3. Check credential health

```console
$ kuberoutectl credential list
ID                                                            PROVIDER  IDENTITY           HEALTH  ACTION
azure:aaaaaaaa-...-000000000001:yeray@example.com             azure     yeray@example.com  valid   use

$ kuberoutectl credential show azure:aaaaaaaa-...:yeray@example.com
ID          azure:aaaaaaaa-...:yeray@example.com
Provider    azure
Identity    yeray@example.com
Health      valid
Action      use
ExpiresAt   2026-07-14T18:32:05Z
```

`ExpiresAt` comes straight from the Azure access token. As it approaches, health
moves `valid` → `expiring` → `expired`, and the suggested action becomes
`renew`.

## 4. Renew when Azure asks you to

```console
$ kuberoutectl credential renew azure:aaaaaaaa-...:yeray@example.com
Renewed credential: azure:aaaaaaaa-...:yeray@example.com
Run `kuberoutectl sync` to refresh health.
```

Under the hood this runs `az login` (scoped to the credential's tenant when one
was recorded). `az login` may open a browser or print a device code — that is
Azure's interactive flow, not something `kuberoutectl` suppresses. After it
completes, re-run `kuberoutectl sync azure` so the cached health reflects the
new token.

## 5. Route kubectl at a cluster

This is the point of the tool — make one AKS cluster your current `kubectl`
context:

```console
$ kuberoutectl target use aks-prod-weu       # the alias — or the full ID
Fetching credentials into ~/.kube/config ...
Now using target: aks-prod-weu (/subscriptions/aaaa.../aks-prod-weu)
kubeconfig updated and set as the current context.
```

By default this runs `az aks get-credentials --subscription <sub>
--resource-group <rg> --name <cluster> --overwrite-existing`, which merges the
cluster into `~/.kube/config` **and** sets it as the current context. Verify:

```bash
kubectl config current-context   # -> aks-prod-weu
kubectl get nodes
```

If you only want to record the selection without touching your kubeconfig
(e.g. on a machine where you don't want to alter `~/.kube/config`):

```bash
kuberoutectl target use <alias|id|name> --no-kubeconfig
```

If the target's credential needs renewal, `target use` warns you and points at
`credential renew`.

## 6. Organize across subscriptions

Discovery attaches **system labels** (provider, region, platform, health).
Add your own **user labels** — they live in the state store and survive every
resync:

```bash
kuberoutectl target label add aks-prod-weu env=prod team=platform
kuberoutectl target label list aks-prod-weu
```

Then save a **collection** — a live view driven by a selector, not a static
folder:

```bash
kuberoutectl collection create prod-eu --selector "env=prod,region=westeurope"
kuberoutectl collection show prod-eu
kuberoutectl collection use prod-eu
```

Selectors match exact values (`env=prod`) and in-lists
(`region in [westeurope, northeurope]`). Beyond your own labels you can select
on structured attributes by bare key: `region`, `platform`, `provider`,
`health`, `kind`. Because collections are label-driven, one collection can span
Azure and AWS (see the [AWS guide](aws.md)).

## Capability summary (Azure)

| Capability          | Azure | Notes                                             |
|---------------------|-------|---------------------------------------------------|
| Discover scopes     | yes   | subscriptions via `az account list`               |
| Credential renew    | yes   | `az login` (interactive)                          |
| Switch context      | yes   | `az aks get-credentials`                          |
| Static credentials  | no    | Azure logins always expire; there is always a renew path |

## Troubleshooting

- **`sync azure` hangs** — expired session waiting on interactive re-auth. Run
  `az login`, then retry.
- **A subscription is missing** — `az account list` only shows subscriptions the
  signed-in identity can see. Check `az account list -o table` directly and your
  directory/tenant.
- **`az` not found** — install the Azure CLI or set an explicit binary path in
  config; `kuberoutectl doctor` shows what it resolved.
- **Wrong cluster after `target use`** — confirm with
  `kubectl config current-context`; re-run `target use` on the intended alias
  (the ALIAS column in `target list` is unique and unambiguous).
