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

## Usage

Every inventory command supports `--output json` (`-o json`) for scripting.

### Commands

```bash
kuberoutectl provider list                       # registered providers + capabilities
kuberoutectl doctor                              # check required provider CLIs resolve

kuberoutectl sync azure                          # discover Azure inventory into the cache
kuberoutectl sync aws                            # discover AWS inventory into the cache

kuberoutectl source list
kuberoutectl scope list
kuberoutectl credential list
kuberoutectl credential show <id>
kuberoutectl credential renew <id>               # if the provider/credential supports it

kuberoutectl target list
kuberoutectl target inspect <id>
kuberoutectl target use <id>

kuberoutectl target label add <target-id> env=prod
kuberoutectl target label remove <target-id> env
kuberoutectl target label list <target-id>

kuberoutectl collection create production --selector env=prod
kuberoutectl collection create eu --selector "region in [westeurope, eu-central-1]"
kuberoutectl collection list
kuberoutectl collection show production
kuberoutectl collection use production
kuberoutectl collection delete production

kuberoutectl version
```

### Example session

Discover both clouds into the local cache:

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

Credential health is a spectrum, not a boolean — note the static AWS key
(`static`/`none`, nothing to renew) versus the expired SSO session
(`expired`/`renew`):

```console
$ kuberoutectl credential list
ID                                                            PROVIDER  IDENTITY                               HEALTH   ACTION
azure:11111111-1111-1111-1111-111111111111:yeray@example.com  azure     yeray@example.com                      valid    use
aws:default                                                   aws                                              expired  renew
aws:legacy-static                                             aws       arn:aws:iam::222222222222:user/ci-bot  static   none
aws:prod-sso                                                  aws       arn:aws:sts::111111111111:assumed-role/AWSReservedSSO_Platform/yeray  valid  use
```

Organize across providers with labels, then collect. Because a collection is a
saved view over labels, one selector can span clouds:

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

User labels are stored separately from discovered inventory, so they survive
`sync` — re-running discovery never erases your organization.

Selectors accept exact matches (`env=prod`, comma-joined or repeated
`--selector`) and simple in-lists (`region in [westeurope, eu-central-1]`).
Beyond your own labels you can select on a target's structured attributes by
bare key: `region`, `platform`, `provider`, `health`, `kind`.

## Building from source

Requires Go (see the `go` directive in `go.mod`). Common tasks are wrapped in
the `Makefile`:

```bash
make build        # build ./bin/kuberoutectl with version info injected
make test         # go test ./...
make check        # format check + vet + test (pre-commit gate)
make dist         # cross-compile windows/amd64 then linux/amd64 into ./dist
make snapshot     # local GoReleaser snapshot build
make help         # list all targets
```

Or directly:

```bash
go build ./cmd/kuberoutectl
go test ./...
```

`kuberoutectl version` reports the injected build metadata (version, commit, date).

## Development workflow

The repository uses:

- `main` for stable code,
- `development` for active integration,
- snapshot CLI builds published from `development` as a mutable GitHub draft
  pre-release for testing.

The `snapshot-release` GitHub Actions workflow builds cross-platform binaries
with GoReleaser on every push to `development` (Windows amd64 primary, Linux
amd64 next) and replaces a single `development-snapshot` draft pre-release. This
makes it easy to develop on a personal machine and validate builds on a more
restricted work environment without promoting every test build to a formal
release.

## License

The project is intended to be open source and is a good fit for **Apache License 2.0**.

## Status

Milestone 1 is implemented: the Azure and AWS providers, JSON local cache,
user labels, and collections all work end to end, with a provider-agnostic core
and cross-platform snapshot builds. See `TODO.md` for what is done and what
remains (multi-region EKS scan, richer selectors, and the future kubeconfig and
GCP providers).

The architecture is shaped around real operator workflows first, not around
generic abstractions for their own sake.
