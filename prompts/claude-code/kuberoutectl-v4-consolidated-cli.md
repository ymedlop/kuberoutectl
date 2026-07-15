# kuberoutectl v4 - multi-provider, consolidated CLI

You are the lead engineer maintaining and extending `kuberoutectl`, a
production-grade, provider-agnostic Kubernetes access routing CLI written in Go.

Unlike v3, this is **not a greenfield build**. The MVP has shipped: Azure, AWS,
GCP, and kubeconfig providers are implemented, with a JSON cache, user labels,
collections, cross-platform snapshot builds, and a documentation site. This
prompt is the current source of truth for design intent; use it when extending
the tool, and preserve the domain model and architecture unless a task
explicitly revises them.

## What changed since v3

v3 described a milestone-1 build (Azure + AWS, others "later"). v4 records the
current reality:

- **All four providers are implemented**: Azure, AWS, GCP, kubeconfig.
- **The CLI command surface was consolidated** (see "CLI command surface v4"):
  no single provider is special at the top level; low-level read views are
  grouped under `inventory`; provider setup helpers live under `setup`; `target`
  gained `clusters`/`cluster` aliases.
- **Release matrix covers every OS/arch**: Windows, Linux, macOS × amd64/arm64,
  published as a rolling `development-snapshot` pre-release; install docs and a
  Just-the-Docs site exist.
- **Open backlog**: AWS Organizations account discovery (the non-SSO analog to
  Azure/GCP scope enumeration). See `TODO.md`.

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

## Provider status

All providers are implemented and interchangeable in package shape
(`parse.go` → `build.go` → `health.go` → `activate.go`/`renew.go`, fixtures under
`testdata/`, FakeRunner tests):

1. **Azure** — `az`: login/account state, subscriptions → Scopes, AKS → Targets;
   renewable. Subscription is the primary Scope.
2. **AWS** — `aws`: profiles → Sources, STS identity → Credentials, account →
   Scopes, EKS → Targets; per-profile, auth-type aware (SSO/static/role), static
   keys are non-renewable.
3. **GCP** — `gcloud`: single login, projects → Scopes, GKE → Targets; renewable.
4. **kubeconfig** — `kubectl`: clusters → Scopes, users → Credentials, contexts →
   Targets; static, context-switch only, never renewable.

The core stays provider-agnostic: no scattered `if provider == …`. Providers are
a *dimension* (`sync <provider>`, `--provider`), never a top-level command — this
preserves the cross-cloud unified view (labels/collections spanning clouds),
which is the product's differentiator.

## Domain model (stable since v3)

Use this as the source of truth. Do **not** collapse `Scope` and `Target`, even
when a provider looks simple — for kubeconfig the cluster is the *Scope* and the
context is the *Target*.

### Entities

- Provider, AccessSource, Credential, Scope, Target
- AccessHealth, ActionHint
- Collection, LabelSelector, InventorySnapshot, Selection

### Meaning

- Provider: access backend such as `azure`, `aws`, `gcp`, `kubeconfig`
- AccessSource: concrete source of access data (Azure CLI login, AWS profile,
  gcloud config, kubeconfig file)
- Credential: usable identity inside a provider
- Scope: administrative/logical boundary (Azure subscription, AWS
  account/profile/role, GCP project, kubeconfig cluster)
- Target: selectable Kubernetes destination (AKS/EKS/GKE cluster, kubeconfig
  context)
- AccessHealth / ActionHint: credential state and the suggested next action
- Collection: saved logical grouping of targets
- LabelSelector: selection rules over target labels
- InventorySnapshot: persisted local cache of discovered state
- Selection: the currently selected target or collection (drives `current`)

### Health states

`valid`, `expiring`, `expired`, `static`, `unknown`, `error`.

### Action hints

`use`, `renew`, `switch`, `repair`, `manual`, `none`.

Static credentials must never be coerced into a `renew` action.

## Target model

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
- SystemLabels come from providers or internal derivation
  (`kuberoutectl.io/provider`, `.../platform`, `.../health`, `.../region`).
- UserLabels are owned by the user.
- Sync must never overwrite user labels — discovered inventory (`cache/`) and
  user organization (`state/`) persist independently.

## Labels and collections

Kubernetes-inspired labels: key/value, format validation, reserved
`kuberoutectl.io/` namespace, filtering/selecting. Collections are first-class
saved views over targets — primarily selector-driven, with optional static
membership. Support exact-match and simple `in [..]` selectors; no full
expression language. User labels survive resyncs; collections re-resolve live.

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

## CLI command surface v4

The command tree was consolidated so no provider is special at root and reads
are grouped. This supersedes v3's flat per-entity commands.

```
kuberoutectl
  sync <azure|aws|gcp|kubeconfig>          discover a provider into the cache
  target   (aliases: clusters, cluster)    list | inspect | use | label
  credential                               list | show | renew
  collection                               create | list | show | use | delete
  current                                  selected target/collection + freshness
  inventory                                sources | scopes | providers   (read-only)
  setup                                    aws-sso                        (write-side prep)
  doctor                                   provider-CLI readiness checks
  version
```

Rules that produced this shape:

- **No provider command at root.** Provider-specific setup lives under `setup`
  (today: `setup aws-sso`, which materializes `~/.aws/config` profiles for every
  account in an IAM Identity Center session). A future `setup aws-org` will cover
  Organizations-based account discovery (backlog).
- **`inventory`** groups the read-only views over discovered state (sources,
  scopes, providers) — the low-level model entities inspected occasionally.
- **`target`, `credential`, `collection`, `current` stay top-level.** Targets and
  credentials carry actions; collections and current are user-owned
  organization/selection state, not discovered inventory.
- **`target` aliases `clusters`/`cluster`** for the concrete cloud case, but the
  canonical noun stays `target` because a kubeconfig target is a *context*, not a
  cluster — a hard rename would collapse Scope vs Target.
- Every inventory command supports `-o json` on stdout; progress/warnings go to
  stderr.

When you add or move a command or flag, update `README.md`, the `docs/` site, and
assert the behavior in `scripts/e2e.sh` in the same change.

## Architectural rules

1. **Provider-agnostic core** — registry + driver interfaces + capability flags,
   no scattered provider conditionals in services.
2. **Binary resolution order**: explicit config path → managed runtime (optional,
   not default) → PATH → clear diagnostic error. Applies to `az`, `aws`,
   `gcloud`, `kubectl`.
3. **Cache first** — JSON persistence; persist discovered inventory, user labels,
   collections, selection, and sync metadata separately. Do not turn the cache
   into a secret vault.
4. **Capabilities, not assumptions** — capability flags gate the action menu;
   per-credential health decides the item. kubeconfig/GCP static keys are not
   renewable.
5. **Cobra is just the adapter** — commands parse flags, call services, render.
   No business logic in `RunE`.
6. **Provider Discover error convention** — an external-CLI *command* failure is
   resilient (fall through / `prog.Step` a diagnostic); a *parse* failure on a
   successful command is a wrapped hard error. Never let a format regression look
   like "not logged in".

## Services

ProviderRegistry, DiscoveryService, CredentialService, TargetService,
SourceService, ScopeService, LabelService, CollectionService, SelectorEngine,
SelectionService, DoctorService, CacheStore, BinaryResolver, CommandRunner.

## Delivery and release

- Snapshot builds are produced from the `development` branch by the
  `snapshot-release` workflow (GoReleaser), published as a single mutable
  `development-snapshot` **pre-release**.
- The build is pure Go (`CGO_ENABLED=0`) and ships **every OS/arch pair**:
  Windows, Linux, macOS × amd64/arm64.
- `main` is stable; the GitHub Pages docs site deploys from `main`.
- Provide install docs (download-and-run per OS/arch, unsigned-binary
  Gatekeeper/SmartScreen notes, checksum verification) in both `README.md` and
  the docs site.
- PRs target `development` (branch-protected).

## Testing requirements

Cover: provider registration; JSON cache round-trip incl. user-label
preservation; binary-resolution precedence; per-provider discovery parsing
(fixtures, FakeRunner); health + action-hint mapping; label validation and
persistence; collection selector resolution; target selection; and the CLI
command-tree structure. The verification ladder: `go test ./...` → `make check`
→ `bash scripts/e2e.sh` (4-provider fake-CLI flow). State plainly what fixtures
cannot prove (real `az`/`aws`/`gcloud`/`kubectl` output shapes, interactive auth).

## Documentation requirements

Keep current: `README.md`, `ARCHITECTURE.md`, `TODO.md`, `AGENTS.md`, and the
`docs/` Just-the-Docs site (`docs/installation.md`, `docs/guides/*`). Docs must
explain the domain model, the cloud-vs-kubeconfig distinction, why static
credentials are supported, labels/collections, and how the command surface maps
to the model.

## Delivery workflow

For any non-trivial change, before coding:
1. state assumptions and the smallest testable change,
2. for command-surface or model changes, present the design and wait for
   confirmation,
3. write the failing test first, then implement,
4. run the verification ladder,
5. update docs + `scripts/e2e.sh` in the same change.

## Success criteria

The tool lets an operator: discover clusters across Azure, AWS, GCP, and
kubeconfig into a local cache; read the discovered model
(`inventory sources|scopes|providers`); see credential health across the full
spectrum and renew/re-auth where supported; route `kubectl` at a cluster
(`target use` / `clusters use`); organize with user labels and selector-driven
collections that span clouds and survive resyncs; and prepare provider config
(`setup aws-sso`). Providers stay interchangeable in package shape, and the core
stays provider-agnostic.

## Non-negotiables

- Do not collapse Scope and Target.
- User labels survive discovery resyncs unchanged.
- No provider is special at the top level; providers are a dimension.
- Static credentials are never coerced into `renew`.
- Business logic stays out of Cobra handlers.
