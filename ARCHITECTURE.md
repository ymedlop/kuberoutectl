# ARCHITECTURE

## Overview

`kuberoutectl` is a provider-agnostic Kubernetes access routing CLI.

Its purpose is to maintain a local, operator-friendly inventory of:
- access providers,
- access sources,
- credentials,
- scopes,
- Kubernetes targets,
- and user-defined organization metadata such as labels and collections.

The CLI is not designed as just a context switcher or a generic version manager. It is designed as a local control plane for access discovery, access health awareness, target organization, and target selection.

## Architectural goals

The architecture should satisfy the following goals:

1. Support multiple providers without spreading provider-specific conditionals across the codebase.
2. Separate discovered state from user-owned organization state.
3. Support both expiring credentials and static credentials.
4. Keep business logic outside the CLI framework layer.
5. Make local cache and inspection first-class capabilities.
6. Preserve room for future providers such as kubeconfig and GCP without redesigning the core model.

## Domain model

The architecture is built around these core entities.

### Provider

Represents a backend of access.

Examples:
- `azure`
- `aws`
- `gcp`
- `kubeconfig`

A provider declares its capabilities and exposes discovery and action behavior through interfaces.

### AccessSource

Represents a concrete source from which access information can be discovered.

Examples:
- Azure CLI login state or profile source
- AWS profile/config source
- future gcloud configuration source
- future kubeconfig file source

This entity allows the architecture to distinguish between the abstract provider and the concrete local source of data.

### Credential

Represents an identity usable inside a provider.

Examples:
- Azure login or account identity
- AWS profile-backed or STS-backed identity
- future gcloud authenticated account
- future kubeconfig user entry

Credentials have health states and action hints.

### Scope

Represents an administrative or logical boundary.

Examples:
- Azure subscription
- Azure tenant boundary when relevant
- AWS account, profile, or role scope
- future GCP project
- future kubeconfig source grouping

Scope is intentionally distinct from Target. Some providers have a strong administrative hierarchy before you reach the actual Kubernetes cluster.

### Target

Represents a Kubernetes destination the user can inspect, label, organize, and use.

Examples:
- AKS cluster
- EKS cluster
- future GKE cluster
- future kubeconfig context

Targets are where user organization becomes most valuable.

### Collection

Represents a saved logical grouping of targets.

Collections are primarily selector-driven and may optionally include static target IDs.

### LabelSelector

Represents a simple selection model over labels.

The first implementation should support:
- exact match labels,
- optional simple “in list” semantics,
- deterministic evaluation.

A full expression language is unnecessary for the initial MVP.

## Access health model

`kuberoutectl` should not reduce all states to just valid or invalid.

The health model should include:

- `valid`
- `expiring`
- `expired`
- `static`
- `unknown`
- `error`

This is important because future kubeconfig-backed credentials may not have a renewable cloud-session lifecycle. A static client certificate or persistent kubeconfig entry should not be forced into a cloud-centric expired/renewed model.

## Action hint model

The architecture should also produce a user-facing next action.

Supported action hints:
- `use`
- `renew`
- `switch`
- `repair`
- `manual`
- `none`

This allows the CLI to be more operator-friendly than a raw inventory dump.

## Separation of discovered state and user state

One of the most important design decisions is the separation between:

- provider-discovered inventory,
- and user-owned organization metadata.

Provider discovery may update:
- sources,
- credentials,
- scopes,
- targets,
- system labels,
- health metadata.

User state should independently persist:
- user labels,
- collections,
- selections,
- preferences.

This separation prevents `sync` operations from overwriting the user’s organization layer.

## Labels and collections

Targets should support two logical label sets.

### System labels

These are discovered or derived by the tool.

Examples:
- `kuberoutectl.io/provider=azure`
- `kuberoutectl.io/platform=aks`
- `kuberoutectl.io/health=valid`
- `kuberoutectl.io/region=westeurope`

### User labels

These are created explicitly by the user.

Examples:
- `env=prod`
- `project=payments`
- `team=platform`
- `owner=yeray`

### Collections as saved views

Collections should not be modeled as simple folders.

Instead, they should act as saved views over targets:
- primarily driven by label selectors,
- optionally extended by static target membership.

This lets newly discovered targets enter a collection automatically when they match the selector.

## Provider architecture

The provider layer should be based on a registry and capability-driven interfaces.

### Required design principles

- The core must not contain scattered `if provider == ...` branches.
- Providers must be pluggable at compile-time through explicit registration.
- Shared services must depend on interfaces, not on provider-specific packages.
- Capabilities must express what a provider supports instead of assuming identical semantics across providers.

### Initial providers

#### Azure

Initial MVP provider.

Responsibilities:
- discover Azure login/account state,
- discover tenants and subscriptions,
- discover AKS clusters,
- map credential health,
- support re-auth or renewal through Azure-native flows.

#### AWS

Second MVP provider.

Responsibilities:
- discover usable profile and identity context,
- validate identity when appropriate,
- discover account/profile/role scopes,
- discover EKS clusters,
- support re-auth or renewal through provider-native flows.

### Future providers

#### kubeconfig

Future provider for self-hosted and local clusters.

This provider differs semantically because:
- access comes from kubeconfig files rather than cloud account hierarchy,
- credentials may be static,
- the operational target is usually the kubeconfig context.

#### GCP

Future provider for GKE and Google Cloud account/project discovery.

## Core services

The architecture should be centered around services rather than CLI handlers.

### ProviderRegistry

Keeps track of registered providers and exposes them to the core.

### DiscoveryService

Coordinates discovery across providers and updates cache state.

### SourceService

Reads and exposes access sources.

### CredentialService

Lists credentials, evaluates health, and triggers renew or re-auth flows when supported.

### TargetService

Lists, inspects, filters, and selects Kubernetes targets.

### LabelService

Validates, attaches, removes, and lists user labels for targets.

### CollectionService

Creates, resolves, and manages saved collections.

### SelectorEngine

Evaluates label selectors against current targets.

### DoctorService

Checks local prerequisites and runtime conditions such as required CLIs or cache integrity.

### BinaryResolver

Resolves the path for required third-party CLIs.

Resolution order should be:
1. explicit config path,
2. managed runtime installed by `kuberoutectl`,
3. PATH lookup,
4. explicit error.

### CommandRunner

Executes external commands in a structured and testable way.

### CacheStore

Abstracts local persistence.

The first implementation should use JSON-based persistence.

## CLI layer

The CLI framework layer should be thin.

Responsibilities of the CLI layer:
- parse flags and arguments,
- call services,
- render text or JSON output,
- return structured errors.

Responsibilities that should not live in the CLI layer:
- discovery logic,
- provider orchestration,
- selector evaluation,
- label mutation rules,
- cache persistence rules.

## Persistence model

The local state should be split by responsibility.

Suggested shape:

```text
~/.kuberoutectl/
  cache/
    providers.json
    sources.json
    credentials.json
    scopes.json
    targets.json
  state/
    user-labels.json
    collections.json
    selections.json
    sync-status.json
```

This structure preserves the separation between discovered inventory and user-owned metadata.

## Development workflow

The repository should support two main working modes:

- active development on a personal machine,
- validation in a restricted work environment.

To support this, the project should publish mutable snapshot builds from the `development` branch using GitHub Actions draft releases.

This makes it possible to continuously test the CLI without turning every integration build into a formal versioned release.

## Testing strategy

The initial test strategy should cover:

- provider registration,
- binary resolution precedence,
- JSON cache read/write behavior,
- discovery parsing for Azure and AWS,
- health state mapping,
- action hint mapping,
- label validation,
- label persistence,
- collection selector resolution,
- target selection behavior.

## Package layout direction

A reasonable package structure is:

```text
cmd/
  kuberoutectl/
internal/
  domain/
  providers/
    registry.go
    azure/
    aws/
    kubeconfig/
    gcp/
  services/
  cache/
    jsonstore/
  execx/
  config/
  cli/
pkg/
```

The exact shape may evolve, but boundaries should remain explicit.

## Non-goals for the first MVP

The first MVP should avoid overreach.

Out of scope initially:
- full plugin runtime architecture,
- broad secret storage features,
- full selector query language,
- deep UI workflows,
- excessive abstraction for providers not yet implemented.

## Summary

The architecture of `kuberoutectl` is designed around a simple principle:

> discover access, understand access health, organize targets, and route the operator to the right cluster with minimal friction.

That principle should remain stable even as providers and capabilities grow.
