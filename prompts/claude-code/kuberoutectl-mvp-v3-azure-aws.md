# kuberoutectl MVP v3 - Azure and AWS

You are the lead engineer implementing a production-grade Go CLI called `kuberoutectl`.

Your job is to design and build the first real implementation of the project, not a toy scaffold.

## Product definition

`kuberoutectl` is a provider-agnostic Kubernetes access routing CLI.

It is:
- a local control plane for Kubernetes access,
- a unified inventory of access sources, credentials, scopes, and targets,
- a cache-backed CLI for discovering and organizing Kubernetes access,
- a user-centric organizer for clusters through labels and collections.

It is not:
- just a wrapper around cloud CLIs,
- just a kubeconfig context switcher,
- just a CLI version manager.

## Core vision

The CLI must support both:
1. cloud-backed Kubernetes access providers,
2. self-hosted/local kubeconfig-based access providers.

The architecture must support:
- providers like Azure, AWS, GCP,
- local kubeconfig sources for self-hosted clusters,
- expiring credentials for cloud providers,
- static or non-expiring credentials for kubeconfig-backed access,
- user-defined labels on targets,
- dynamic and static collections of targets.

We will implement only:
- Azure provider,
- AWS provider,
- user labels,
- collections,
- JSON local cache,
- snapshot-release-friendly build shape.

in milestone 1.

Then add later:
- kubeconfig provider,
- GCP provider,
- richer selectors,
- optional managed runtime support for third-party CLIs.

However, the architecture must be ready for future providers without changing the core model.

## Domain model v3

Use this domain model as the source of truth.

### Main entities

- Provider
- AccessSource
- Credential
- Scope
- Target
- AccessHealth
- ActionHint
- Collection
- LabelSelector
- InventorySnapshot

### Conceptual meaning

- Provider: access backend such as `azure`, `aws`, `gcp`, `kubeconfig`
- AccessSource: concrete source of access data, such as an Azure CLI profile, AWS profile/config, gcloud configuration, or kubeconfig file
- Credential: usable identity inside a provider
- Scope: administrative or logical grouping, such as Azure subscription, AWS account/profile/role scope, GCP project, or kubeconfig source group
- Target: selectable Kubernetes destination, usually a managed cluster or a kubeconfig context
- AccessHealth: state of the credential or target
- ActionHint: what the user should do next
- Collection: saved logical grouping of targets
- LabelSelector: selection rules over target labels
- InventorySnapshot: persisted local cache of discovered state

### Health states

Use these health states:
- valid
- expiring
- expired
- static
- unknown
- error

### Action hints

Use these action hints:
- use
- renew
- switch
- repair
- manual
- none

## Target model

Targets must support both:
- system-discovered labels,
- user-defined labels.

Use a structure close to this:

```go
type Target struct {
    ID           TargetID
    ProviderID   ProviderID
    SourceID     SourceID
    CredentialID CredentialID
    ScopeID      ScopeID
    Kind         string
    Name         string
    Endpoint     string
    Region       string
    Platform     string
    Health       AccessHealth
    ActionHint   ActionHint
    LastSeenAt   time.Time

    SystemLabels map[string]string
    UserLabels   map[string]string

    Metadata     map[string]string
}
```

Rules:
- SystemLabels come from providers or kuberoutectl internal derivation.
- UserLabels are owned by the user.
- Sync must never overwrite user labels.
- User organization state must be persisted independently from provider discovery state.

## Labels

Implement labels inspired by Kubernetes label semantics:
- key/value pairs,
- format validation,
- multiple labels per target,
- filtering/selecting targets by labels.

Use a reserved internal label namespace such as:
- `kuberoutectl.io/provider`
- `kuberoutectl.io/source`
- `kuberoutectl.io/health`
- `kuberoutectl.io/platform`

Do not mix internal system labels and user labels carelessly.

## Collections

Collections are user-defined groupings of targets.

They should support:
1. selector-based dynamic membership,
2. optional static membership by explicit target ID.

Use a model close to this:

```go
type Collection struct {
    ID          CollectionID
    Name        string
    Description string
    Selector    LabelSelector
    StaticIDs   []TargetID
    Metadata    map[string]string
}

type LabelSelector struct {
    MatchLabels map[string]string
    MatchAny    map[string][]string
}
```

For milestone 1:
- support exact-match selectors,
- optionally support simple `in list` semantics,
- do not build a full expression language.

## Provider behavior expectations

### Azure provider

For Azure:
- discover login and account state from Azure CLI,
- discover tenants and subscriptions,
- discover AKS clusters per relevant subscription,
- evaluate credential health as far as realistically possible,
- support renewal or re-auth orchestration through Azure CLI login/account flows,
- treat subscription as the primary Scope abstraction.

### AWS provider

For AWS:
- discover usable profiles and/or identity context,
- validate effective identity via STS where appropriate,
- discover account/profile/role scopes needed for access routing,
- discover EKS clusters,
- evaluate credential health as far as realistically possible,
- support renewal or re-auth orchestration through provider-native flows such as SSO/profile refresh.

### Later providers

#### kubeconfig provider
- discover kubeconfig sources from explicit config paths, KUBECONFIG, and default ~/.kube/config,
- discover contexts, clusters, and users from kubeconfig,
- model kubeconfig contexts as Targets,
- use kubectl-native semantics for switching targets,
- treat many kubeconfig credentials as `static`,
- support health checks where possible,
- allow actions like `switch`, `repair`, or `manual`,
- warn on potentially untrusted kubeconfig sources.

#### GCP provider
- discover credentialed accounts from gcloud,
- discover config/configuration state,
- discover projects/scopes,
- discover GKE clusters,
- evaluate credential health,
- support renewal orchestration via gcloud auth flows.

## Important real-world constraints

- Azure should be milestone 1 because it is a real target environment for operator usage.
- AWS should be milestone 2 for the same reason.
- The architecture must still be provider-agnostic despite starting with Azure and AWS.
- Kubernetes labels are key/value metadata intended for organization and selection.
- Future kubeconfig support must not force cloud semantics onto static credentials.

Do not ignore these constraints.

## Architectural rules

### 1. Provider-agnostic core

The core must not contain scattered provider-specific conditionals.
Use:
- provider registry,
- driver interfaces,
- capability-based behavior.

### 2. Binary resolution

Third-party CLI management is optional, not default.

Binary resolution order must be:
1. explicit path from config,
2. managed runtime installed by kuberoutectl,
3. PATH lookup,
4. clear diagnostic error.

This applies to:
- `az`
- `aws`
- `kubectl`
- future provider CLIs.

### 3. Local cache first

The CLI must maintain a local cache of discovered inventory.

For milestone 1, use JSON persistence.

Persist separately:
- discovered inventory,
- user labels,
- collections,
- selection state,
- sync metadata.

Do not persist cloud secrets unless absolutely necessary.

### 4. Capabilities, not assumptions

Not every provider supports the same actions.

Examples:
- Azure supports renewal semantics,
- AWS supports renewal or re-auth flows depending on auth type,
- kubeconfig may support static credentials and context switching but not renewal.

### 5. Cobra is just the adapter

Business logic must not live in Cobra commands.

## Required services

Implement services with clear separation:
- ProviderRegistry
- DiscoveryService
- CredentialService
- TargetService
- SourceService
- LabelService
- CollectionService
- SelectorEngine
- DoctorService
- CacheStore
- BinaryResolver
- CommandRunner

## Required CLI commands for milestone 1

Implement these commands or close equivalents:

- `kuberoutectl provider list`
- `kuberoutectl source list`
- `kuberoutectl credential list`
- `kuberoutectl credential show <id>`
- `kuberoutectl credential renew <id>`
- `kuberoutectl scope list`
- `kuberoutectl target list`
- `kuberoutectl target inspect <id>`
- `kuberoutectl target use <id>`
- `kuberoutectl target label add <target-id> <key=value>`
- `kuberoutectl target label remove <target-id> <key>`
- `kuberoutectl target label list <target-id>`
- `kuberoutectl collection create <name> --selector <expr>`
- `kuberoutectl collection list`
- `kuberoutectl collection show <name>`
- `kuberoutectl collection use <name>`
- `kuberoutectl sync azure`
- `kuberoutectl sync aws`
- `kuberoutectl doctor`

Add JSON output support.

## Testing requirements

Write tests for:
- provider registration,
- JSON cache persistence,
- target selection logic,
- binary resolution precedence,
- Azure discovery parsing logic,
- AWS discovery parsing logic,
- health state mapping,
- action hint mapping,
- label validation,
- label persistence,
- collection selector resolution.

## Documentation requirements

Create:
- `README.md`
- `ARCHITECTURE.md`
- `TODO.md`

The docs must explain:
- the domain model,
- the difference between cloud providers and future kubeconfig provider,
- why static credentials are supported,
- how labels and collections work,
- how future GCP and kubeconfig providers would fit in,
- why managed runtimes are optional.

## Delivery workflow

Before coding:
1. show the proposed file tree,
2. explain the domain model,
3. explain provider capability design,
4. explain label and collection design,
5. explain how Azure and AWS differ semantically,
6. explain how future kubeconfig and GCP would plug in,
7. then implement.

Do not skip the design step.

## Implementation bias

Prefer:
- small, real, testable implementation,
- explicit code over reflection,
- JSON cache over SQLite for milestone 1,
- deterministic behavior over magic auto-discovery,
- provider-specific drivers over giant shared conditionals.

Avoid:
- fake abstractions,
- plugin systems,
- over-engineering,
- hidden runtime behavior in Cobra commands,
- forcing cloud semantics onto future kubeconfig support,
- storing user organization state inside ephemeral discovery pipelines.

## Success criteria

At the end of milestone 1, I should be able to:
- list providers,
- discover Azure tenants, subscriptions, and AKS clusters,
- discover AWS profiles, identity context, and EKS clusters,
- see health states including renewable or re-auth states,
- renew or re-auth a credential when supported,
- label targets,
- create collections from labels,
- inspect the local cached inventory,
- understand clearly from the code how kubeconfig and GCP would be added next.

## Important note

- Collections are first-class saved views over targets, primarily driven by labels/selectors, with optional static additions.
- User labels must survive discovery resyncs unchanged.
- Do not collapse Scope and Target into one type just because some providers appear simpler.
- Start with Windows amd64 as the primary deliverable for snapshot builds, then add Linux amd64.

Now begin by showing:
1. the final file tree,
2. the domain model summary,
3. the key interfaces,
4. the capability matrix for Azure vs AWS,
5. the label and collection model,
6. and only then start implementation.
