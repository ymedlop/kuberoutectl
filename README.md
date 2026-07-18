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

## What 1.0.0 includes

Everything the stable release ships (see [CHANGELOG.md](CHANGELOG.md) for detail):

- Azure, AWS, GCP, and kubeconfig providers on a provider-agnostic core
- JSON local cache with labels + collections that survive resync
- Credential-health awareness and target visibility
- Cross-platform distribution: archives + checksums, Homebrew, Scoop, deb/rpm/apk
- Documented, reproducible, repeatable release automation

## Roadmap

Post-1.0 work — additive, and it does not change the core workflow:

- managed `kubectl` runtime with version compatibility + selection ([#37](https://github.com/ymedlop/kuberoutectl/issues/37)–[#42](https://github.com/ymedlop/kuberoutectl/issues/42))
- an MCP server for `kuberoutectl` ([#44](https://github.com/ymedlop/kuberoutectl/issues/44))
- an AI-skills repo ([#43](https://github.com/ymedlop/kuberoutectl/issues/43))
- richer health checks and improved collection selectors

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

### Working with labels and collections

Because a collection is a **live query over labels**, not a static folder, you
can create it first and tag clusters into it later — order does not matter.

```bash
# 1. Create the collection with a selector (0 members is fine — nothing matches yet)
kuberoutectl collection create production --selector env=prod

# 2. Label clusters whenever you like — they join automatically
kuberoutectl target list                          # find the ALIAS to reference
kuberoutectl target label add aks-prod-weu       env=prod
kuberoutectl target label add eks-prod-frankfurt env=prod

# 3. Membership re-resolves live — no resync needed
kuberoutectl collection show production           # Members: 2

# 4. Point kubectl at the whole set
kuberoutectl collection use production
```

Key properties:

- **Order-independent** — label a new cluster tomorrow and it appears in
  `production` with no extra step; the collection re-resolves from current
  labels every time.
- **Survives discovery** — user labels are never wiped by `sync`, so
  collections keep matching across resyncs.
- **Cross-cloud** — one selector (`env=prod`) spans Azure, AWS, GCP, and
  kubeconfig at once.
- **Selectors** accept exact matches (`env=prod`), comma-joined or repeated
  `--selector`, and `in` lists (`"region in [westeurope, eu-central-1]"`). You
  can also select on a target's structured attributes by bare key: `region`,
  `platform`, `provider`, `health`, `kind`.
- **Static members** — add one-offs that don't fit a selector at creation with
  `--static <target-id>` (unioned with the selector matches).
- **Manage labels** — `target label list <ref>` to see them,
  `target label remove <ref> <key>` to drop one. `kuberoutectl.io/` is a
  reserved system namespace; your labels are plain `key=value`.

A fuller walkthrough lives in the docs site under
[Organizing: labels & collections](docs/organizing.md).

## Installation

Pre-built binaries come in two flavors: **stable** releases cut from `vX.Y.Z`
tags, and a rolling **`development-snapshot`** pre-release rebuilt on every push
to `development`. Grab either from the releases page:

**→ [github.com/ymedlop/kuberoutectl/releases](https://github.com/ymedlop/kuberoutectl/releases)**

See **[RELEASING.md](RELEASING.md)** for how releases are produced and verified.

Each build ships **Windows, Linux, and macOS in both `amd64` and `arm64`**.
Archives are named `kuberoutectl_<version>_<os>_<arch>.<ext>` (`.tar.gz` for Linux
and macOS, `.zip` for Windows), plus `.deb`/`.rpm`/`.apk` Linux packages. Not sure
which architecture you need? Run `uname -m` — `x86_64` → `amd64`,
`aarch64`/`arm64` → `arm64`.

Use a **package manager** (below) for automatic upgrades, or **download the
archive directly** if you'd rather manage a single binary. Install command not
working? See the
[installation guide's Troubleshooting](https://ymedlop.github.io/kuberoutectl/installation/#troubleshooting).

### macOS (Homebrew)

```bash
brew install ymedlop/tap/kuberoutectl
```

Installs and updates via `brew`, no manual quarantine step. Manual download works
too — see below.

### Linux (packages)

```bash
sudo dpkg -i kuberoutectl_*_amd64.deb                       # Debian/Ubuntu
sudo rpm -i  kuberoutectl_*_amd64.rpm                        # Fedora/RHEL
sudo apk add --allow-untrusted kuberoutectl_*_amd64.apk     # Alpine
```

`.deb`/`.rpm`/`.apk` (amd64 + arm64) ship as release assets; the binary lands at
`/usr/bin/kuberoutectl`.

### Linux and macOS (manual)

```bash
# Download the asset for your OS (linux|darwin) and arch (amd64|arm64) from the
# releases page, then — from the folder where it landed:
tar -xzf kuberoutectl_*_linux_amd64.tar.gz      # adjust os/arch to match
chmod +x kuberoutectl
sudo mv kuberoutectl /usr/local/bin/             # or any dir on your PATH
kuberoutectl version
```

On **macOS** the binary is unsigned, so Gatekeeper quarantines it on first run.
Clear the quarantine flag once after extracting:

```bash
xattr -d com.apple.quarantine ./kuberoutectl     # or: right-click → Open
```

### Windows (Scoop)

```powershell
scoop bucket add ymedlop https://github.com/ymedlop/scoop-bucket
scoop install kuberoutectl
```

Installs and updates via [Scoop](https://scoop.sh). Manual download works too —
see below.

### Windows (manual)

Download the `..._windows_<arch>.zip` asset, extract it, and run from PowerShell:

```powershell
Expand-Archive kuberoutectl_*_windows_amd64.zip -DestinationPath kuberoutectl
.\kuberoutectl\kuberoutectl.exe version
```

Move `kuberoutectl.exe` somewhere on your `PATH` to call it from anywhere.
SmartScreen may warn about the unsigned binary — choose **More info → Run
anyway**.

### Verify the download (optional)

Each release includes `checksums.txt`:

```bash
sha256sum -c checksums.txt          # Linux
shasum -a 256 -c checksums.txt      # macOS
# Windows (PowerShell): Get-FileHash .\kuberoutectl_*.zip -Algorithm SHA256
```

Prefer to build it yourself? See [Building from source](#building-from-source).

## Usage

Every inventory command supports `--output json` (`-o json`) for scripting.

### Commands

```bash
kuberoutectl doctor                              # check required provider CLIs resolve

kuberoutectl sync azure                          # discover Azure inventory into the cache
kuberoutectl sync aws                            # discover AWS inventory into the cache
kuberoutectl sync gcp                            # discover GCP (GKE) inventory into the cache
kuberoutectl sync kubeconfig                     # discover kubeconfig contexts (contexts duplicating a natively-synced cluster, by endpoint, are suppressed)

kuberoutectl inventory providers                 # registered providers + capabilities
kuberoutectl inventory sources                   # discovered access sources
kuberoutectl inventory scopes                    # discovered scopes (subscriptions/accounts/projects)

kuberoutectl credential list
kuberoutectl credential list --provider aws      # filter by provider
kuberoutectl credential show <id>
kuberoutectl credential renew <id>               # if the provider/credential supports it

kuberoutectl target list                         # short ALIAS column, not the long ID
kuberoutectl clusters list                       # `clusters`/`cluster` are aliases of `target`
kuberoutectl target list --provider aws          # filter by provider
kuberoutectl target list -l env=prod             # filter by selector (repeatable)
kuberoutectl target list --wide                  # also show the full ID
kuberoutectl target inspect <alias|id|name>          # details incl. Kubernetes server version (unknown for kubeconfig)
kuberoutectl target use <alias|id|name>              # fetch credentials into ~/.kube/config + set context
kuberoutectl target use <alias|id|name> --no-kubeconfig  # record the selection only

kuberoutectl target delete <alias|id|name>           # drop one target from the cache (a resync re-adds it)
kuberoutectl target clear                            # drop all targets (prompts; --yes to skip); a resync repopulates

kuberoutectl target hide <alias|id|name>             # hide from the default list; persists across resyncs
kuberoutectl target hide -l env=staging              # hide every matching target (bulk, by selector)
kuberoutectl target unhide <alias|id|name>           # reveal a hidden target again
kuberoutectl target list --all                       # include hidden targets (adds a HIDDEN column)
kuberoutectl target list -l hidden=true              # list only hidden targets

kuberoutectl target label add <alias|id|name> env=prod
kuberoutectl target label remove <alias|id|name> env
kuberoutectl target label list <alias|id|name>

kuberoutectl collection create production --selector env=prod
kuberoutectl collection create eu --selector "region in [westeurope, eu-central-1]"
kuberoutectl collection list
kuberoutectl collection show production
kuberoutectl collection use production
kuberoutectl collection delete production

kuberoutectl current                             # what am I pointed at, and how fresh is it?

kuberoutectl setup aws-sso --sso-session <name>  # write ~/.aws/config profiles for every SSO account
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

### Provider guides

Step-by-step manuals for managing clusters and credentials on each cloud:

- [Azure (AKS)](docs/guides/azure.md)
- [AWS (EKS)](docs/guides/aws.md) — including corporate IAM Identity Center / Entra sign-in
- [GCP (GKE)](docs/guides/gcp.md)
- [kubeconfig](docs/guides/kubeconfig.md) — self-hosted / local / handed-to-you contexts

See [docs/guides/](docs/guides/index.md) for the shared model and the
credential-health spectrum.

The same guides, with search and navigation, are published at
**[ymedlop.github.io/kuberoutectl](https://ymedlop.github.io/kuberoutectl/)**.

## Building from source

Requires Go (see the `go` directive in `go.mod`). Common tasks are wrapped in
the `Makefile`:

```bash
make build        # build ./bin/kuberoutectl with version info injected
make test         # go test ./...
make check        # format check + vet + test (pre-commit gate)
make dist         # cross-compile {windows,linux,darwin} × {amd64,arm64} into ./dist
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
- snapshot CLI builds published from `development` as a mutable GitHub
  pre-release for testing.

The `snapshot-release` GitHub Actions workflow builds cross-platform binaries
with GoReleaser on every push to `development` — Windows, Linux, and macOS, each
in `amd64` and `arm64` — and replaces a single `development-snapshot`
pre-release. This makes it easy to develop on a personal machine and validate
builds on a more restricted work environment without promoting every test build
to a formal release.

## License

The project is intended to be open source and is a good fit for **Apache License 2.0**.

## Status

**1.0.0 is the first stable public release — a stability milestone, not a feature
milestone.** The core discover → organize → route workflow is complete across the
Azure, AWS, GCP, and kubeconfig providers, with a provider-agnostic core, JSON
local cache, user labels and collections that survive resync, credential-health
awareness, and cross-platform package distribution (Homebrew, Scoop, deb/rpm/apk).
The command surface is not expected to change in breaking ways. See
[CHANGELOG.md](CHANGELOG.md) for the full 1.0.0 summary and
[RELEASING.md](RELEASING.md) for the release process.

Post-1.0 work (a managed `kubectl` runtime with version selection, an MCP server,
and an AI-skills repo) is tracked as roadmap and does not change the core
workflow — see [Roadmap](#roadmap). `TODO.md` is the historical milestone-1
tracker, kept for reference.

The architecture is shaped around real operator workflows first, not around
generic abstractions for their own sake.
