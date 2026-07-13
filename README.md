# kuberoutectl

`kuberoutectl` is an open source CLI to discover, organize, and use Kubernetes clusters across cloud providers and future self-hosted sources.

The project starts with **Azure first** and **AWS next**, because those are the most relevant real-world target environments for the initial MVP. The long-term goal is a provider-agnostic access routing layer for Kubernetes that keeps a local inventory of access sources, credentials, scopes, and targets, then helps the user understand what they can access, what is expired, and what action to take next.

## Why this exists

Working across multiple Kubernetes clusters is usually fragmented.

Operators often need to move between:
- multiple cloud providers,
- multiple identities or subscriptions/accounts,
- multiple clusters per environment,
- and different local access methods.

The current toolchain usually gives you pieces of the workflow, not the whole flow. One CLI may help you authenticate, another may switch contexts, and another may help inspect a cluster, but there is often no single operator-focused layer that keeps an organized local inventory of access and lets you route to the right cluster quickly.

`kuberoutectl` is meant to fill that gap.

## Project goals

The initial goals of the project are:

- Discover Kubernetes access targets from supported providers.
- Cache discovered inventory locally.
- Detect whether a credential is valid, expiring, expired, static, or unknown.
- Help the user renew or re-authenticate credentials when supported.
- Let the user organize targets with labels.
- Let the user create collections such as `env=prod`, `project=payments`, or `team=platform`.
- Keep provider logic behind a provider-agnostic core.

## MVP scope

### Milestone 1

- Azure provider
- AWS provider
- Local cache in JSON
- Labels on targets
- Collections built from labels
- Snapshot builds from the `development` branch via GitHub Actions draft releases

### Later milestones

- kubeconfig provider for self-hosted and local clusters
- GCP provider
- richer health checks
- improved selector support for collections
- optional managed runtime support for third-party CLIs

## Core concepts

The CLI is built around a stable domain model:

- **Provider**: source of access such as `azure`, `aws`, `gcp`, or `kubeconfig`.
- **AccessSource**: concrete source of access data, such as an Azure CLI profile, AWS profile, or kubeconfig file.
- **Credential**: usable identity inside a provider.
- **Scope**: administrative or logical boundary, such as an Azure subscription or AWS account/profile scope.
- **Target**: selectable Kubernetes destination, usually a managed cluster or future kubeconfig context.
- **Labels**: key/value metadata used to organize targets.
- **Collections**: saved logical views over targets.

## Design principles

- **Provider-agnostic core**: provider-specific logic stays behind interfaces and a registry.
- **User-owned organization**: user labels and collections must survive discovery resyncs.
- **Cache first**: the CLI keeps a local inventory to make access easier to inspect and navigate.
- **No secret vault by default**: the cache is not intended to become a general secret store.
- **Optional CLI management**: third-party CLI management may exist later, but it is not the default model.
- **Operator-focused UX**: the tool should help answer practical questions quickly, such as “what clusters do I have?”, “which credential is expired?”, and “what should I use next?”.

## Labels and collections

Kubernetes labels are a strong inspiration for the organization layer of `kuberoutectl`.

Targets can have:
- **system labels**, discovered or derived by the tool,
- **user labels**, defined by the operator.

Collections are not just static folders. They are saved views over targets, primarily driven by labels and selectors, with optional static additions when needed.

Examples:
- `production` → `env=prod`
- `lab` → `env=lab`
- `platform-eu` → `team=platform` and `region in [westeurope, eu-west-1]`

## Planned command shape

The exact UX may evolve, but the current direction looks like this:

```bash
kuberoutectl provider list
kuberoutectl source list
kuberoutectl credential list
kuberoutectl scope list
kuberoutectl target list
kuberoutectl target inspect <id>
kuberoutectl target use <id>

kuberoutectl target label add <target-id> env=prod
kuberoutectl target label remove <target-id> env
kuberoutectl target label list <target-id>

kuberoutectl collection create production --selector env=prod
kuberoutectl collection list
kuberoutectl collection show production
kuberoutectl collection use production

kuberoutectl sync azure
kuberoutectl sync aws
kuberoutectl doctor
```

## Development workflow

The repository is expected to use:

- `main` for stable code,
- `development` for active integration,
- snapshot CLI builds published from `development` as a mutable GitHub draft release for testing.

This makes it easier to develop on a personal machine and validate builds on a more restricted work environment without promoting every test build to a formal release.

## License

The project is intended to be open source and is a good fit for **Apache License 2.0**.

## Status

This repository is in early design and MVP implementation stage.

The architecture is being shaped around real operator workflows first, not around generic abstractions for their own sake.
