# kuberoutectl Documentation

Welcome to the documentation for **kuberoutectl**, an open source CLI to discover, organize, and use Kubernetes clusters across cloud providers and future self-hosted sources.

## About kuberoutectl

`kuberoutectl` is built to solve a real operational problem: **managing Kubernetes access across multiple cloud providers is fragmented**.

### The Problem

Operators often need to move between:
- Multiple cloud providers (Azure, AWS, GCP, self-hosted)
- Multiple identities or subscriptions/accounts
- Multiple clusters per environment
- Different local access methods

The current toolchain gives you pieces — one CLI for auth, another for context switching, another for inspection — but no single operator-focused layer that keeps an organized local inventory of access and lets you route to the right cluster quickly.

### The Solution

`kuberoutectl` fills that gap by:

- **Discovering** Kubernetes access targets from supported providers (Azure, AWS, GCP, kubeconfig)
- **Caching** discovered inventory locally for quick access
- **Detecting** credential health — valid, expiring, expired, static, or unknown
- **Helping** users renew or re-authenticate credentials when supported
- **Organizing** targets with user-defined labels and collections
- **Keeping** provider logic behind a provider-agnostic core

## Quick Start

If you're already familiar with `kuberoutectl`, here's the universal workflow:

```bash
kuberoutectl doctor              # 1. is the provider CLI reachable?
kuberoutectl sync <provider>     # 2. discover clusters + credential health
kuberoutectl credential list     # 3. what's valid / expiring / expired?
kuberoutectl target list         # 4. what can I reach?
kuberoutectl target use <id>     # 5. route kubectl at one cluster
```

## Core Concepts

The CLI is built around a stable domain model that works identically across all providers:

- **Provider**: source of access such as `azure`, `aws`, `gcp`, or `kubeconfig`
- **AccessSource**: concrete source of access data (Azure CLI profile, AWS profile, kubeconfig file)
- **Credential**: usable identity inside a provider
- **Scope**: administrative or logical boundary (subscription, account, project)
- **Target**: selectable Kubernetes destination (AKS, EKS, GKE, or kubeconfig context)
- **Labels**: key/value metadata used to organize targets
- **Collections**: saved logical views over targets, driven by label selectors

## Documentation Structure

### [Provider Guides](guides/)

Step-by-step manuals for using `kuberoutectl` with each supported cloud:

- **[Azure (AKS)](guides/azure.md)** — managing AKS clusters and credentials with Azure CLI
- **[AWS (EKS)](guides/aws.md)** — managing EKS clusters across profiles and accounts
- **[GCP (GKE)](guides/gcp.md)** — managing GKE clusters with gcloud
- **[kubeconfig](guides/kubeconfig.md)** — self-hosted, local, and handed-to-you contexts

Each guide covers:
1. **Setting up the provider** — ensuring your CLI is configured and authenticated
2. **Discovering clusters** — using `sync` to populate the local cache
3. **Checking credential health** — understanding what's valid, expiring, or expired
4. **Managing clusters** — inspecting, selecting, and routing to targets
5. **Organizing with labels** — tagging clusters for easy filtering
6. **Creating collections** — saving views with selectors

### [Shared Model](guides/README.md)

The guides reference a shared domain model that lets the same commands work identically across all providers. This section explains:
- How each cloud provider maps to the universal model
- The credential health spectrum
- The universal workflow loop

## Common Commands

Every inventory command supports `--output json` (`-o json`) for scripting.

```bash
# Providers and setup
kuberoutectl provider list                    # registered providers + capabilities
kuberoutectl doctor                           # check required provider CLIs resolve

# Discovery
kuberoutectl sync azure                       # discover Azure inventory
kuberoutectl sync aws                         # discover AWS inventory
kuberoutectl sync gcp                         # discover GCP inventory
kuberoutectl sync kubeconfig                  # discover kubeconfig contexts

# Credentials
kuberoutectl credential list                  # list all credentials with health status
kuberoutectl credential list --provider aws  # filter by provider
kuberoutectl credential show <id>             # show credential details
kuberoutectl credential renew <id>            # renew a credential if supported

# Targets (Clusters)
kuberoutectl target list                      # list clusters with health
kuberoutectl target list --provider aws       # filter by provider
kuberoutectl target list -l env=prod          # filter by label selector
kuberoutectl target inspect <alias|id|name>  # detailed cluster info
kuberoutectl target use <alias|id|name>      # activate a cluster (update kubeconfig)

# Labels
kuberoutectl target label add <id> env=prod           # add labels
kuberoutectl target label remove <id> env             # remove labels
kuberoutectl target label list <id>                   # list labels

# Collections
kuberoutectl collection create prod --selector env=prod            # save a labeled view
kuberoutectl collection list                                       # list saved collections
kuberoutectl collection show prod                                  # show collection members
kuberoutectl collection use prod                                   # activate all targets in a collection

# Status
kuberoutectl current                          # what am I pointed at?
kuberoutectl version                          # show version info
```

## Architecture & Design Principles

- **Provider-agnostic core**: provider-specific logic stays behind interfaces
- **User-owned organization**: labels and collections survive discovery resyncs
- **Cache first**: local inventory for fast access and organization
- **No secret vault**: the cache stores inventory, not credentials
- **Operator-focused UX**: answers practical questions quickly

For deeper architectural details, see the main [README.md](../README.md) or [ARCHITECTURE.md](../ARCHITECTURE.md).

## Example Workflow

### Discover both Azure and AWS

```console
$ kuberoutectl sync azure && kuberoutectl sync aws
Synced provider: azure
  sources:     1
  credentials: 1
  scopes:      3
  targets:     3
Synced provider: aws
  sources:     3
  credentials: 3
  scopes:      2
  targets:     2
```

### Check credential health across clouds

```console
$ kuberoutectl credential list
ID                                                            PROVIDER  IDENTITY                               HEALTH   ACTION
azure:11111111-1111-1111-1111-111111111111:yeray@example.com  azure     yeray@example.com                      valid    use
aws:default                                                   aws                                              expired  renew
aws:legacy-static                                             aws       arn:aws:iam::222222222222:user/ci-bot  static   none
aws:prod-sso                                                  aws       arn:aws:sts::111111111111:assumed-role/AWSReservedSSO_Platform/yeray  valid  use
```

### Organize with labels and collections

```console
$ kuberoutectl target label add <aks-cluster-id> env=prod
$ kuberoutectl target label add <eks-cluster-id> env=prod

$ kuberoutectl collection create production --selector env=prod
Created collection: production

$ kuberoutectl collection show production
Collection: production
Members: 2
aks-prod-weu        aks  westeurope    valid
eks-prod-frankfurt  eks  eu-central-1  valid
```

## Getting Help

- **New to kuberoutectl?** Start with the [Quick Start](#quick-start) and a provider guide for your cloud.
- **Setting up a specific cloud?** Jump to [Azure](guides/azure.md), [AWS](guides/aws.md), [GCP](guides/gcp.md), or [kubeconfig](guides/kubeconfig.md).
- **Understanding credential health?** See [Credential Health, Once](guides/README.md#credential-health-once).
- **Advanced workflows?** Check the [Common Commands](#common-commands) section or the main [README](../README.md).

## Contributing

`kuberoutectl` is open source. For source code, building, and development workflow, see the main [README.md](../README.md) and [ARCHITECTURE.md](../ARCHITECTURE.md).

## License

Apache License 2.0. See [LICENSE](../LICENSE) for details.
