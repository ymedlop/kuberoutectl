# kuberoutectl

[![Go](https://img.shields.io/github/go-mod/go-version/ymedlop/kuberoutectl?logo=go&label=Go)](go.mod)
[![Release](https://img.shields.io/github/v/release/ymedlop/kuberoutectl?logo=github&label=release)](https://github.com/ymedlop/kuberoutectl/releases)
[![CI](https://github.com/ymedlop/kuberoutectl/actions/workflows/ci.yml/badge.svg)](https://github.com/ymedlop/kuberoutectl/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/ymedlop/kuberoutectl)](LICENSE)
[![Providers](https://img.shields.io/badge/providers-AKS%20%C2%B7%20EKS%20%C2%B7%20GKE%20%C2%B7%20kubeconfig-informational)](#provider-guides)

**Discover, organize, and route Kubernetes access across Azure, AWS, GCP, and
kubeconfig — from one operator-focused CLI.** `kuberoutectl` keeps a local
inventory of your access sources, credentials, scopes, and targets, tells you
what is valid or expired, and points `kubectl` at the right cluster.

![kuberoutectl demo: sync four providers, list targets, inspect credential health, label and collect, then route kubectl](assets/demo.gif)

## Quickstart

Install (full matrix — Homebrew, apt, Scoop, packages, manual — in the
[installation guide](https://ymedlop.github.io/kuberoutectl/installation/)):

```bash
brew install ymedlop/tap/kuberoutectl                                  # macOS
curl -1sLf 'https://dl.cloudsmith.io/public/ymedlop/kuberoutectl/setup.deb.sh' | sudo bash && sudo apt install kuberoutectl   # Debian/Ubuntu
scoop bucket add ymedlop https://github.com/ymedlop/scoop-bucket && scoop install kuberoutectl   # Windows
```

Then run the discover → organize → route loop:

```bash
kuberoutectl doctor              # are the provider CLIs reachable?
kuberoutectl sync azure          # discover clusters + credential health (also: aws | gcp | kubeconfig)
kuberoutectl target list         # what can I reach, and is it healthy?
kuberoutectl target use <alias>  # fetch credentials into ~/.kube/config and switch context
```

`target list` gives you the routable clusters and their health at a glance:

```console
$ kuberoutectl target list
ALIAS               PLATFORM    REGION          HEALTH  PROVIDER
aks-prod-weu        aks         westeurope      valid   azure
eks-prod-frankfurt  eks         eu-central-1    valid   aws
gke-prod-euw1       gke         europe-west1    valid   gcp
homelab             kubeconfig                  static  kubeconfig
```

The demo above shows the fuller flow — credential-health inspection, labels, and
collections across all four providers. Everything below is depth: how it's
organized, every command, and per-cloud guides.

## Why kuberoutectl

Working across multiple Kubernetes clusters is fragmented. Operators move between
multiple clouds, multiple identities or subscriptions/accounts, multiple clusters
per environment, and different local access methods. The usual toolchain gives
you pieces — one CLI authenticates, another switches contexts, another inspects a
cluster — but no single operator-focused layer that keeps an organized local
inventory of access and routes you to the right cluster quickly.

`kuberoutectl` fills that gap. What makes it different:

- **Provider-agnostic core** — Azure, AWS, GCP, and kubeconfig behind one model;
  provider-specific logic stays behind interfaces and a registry.
- **Credential health is a spectrum, not a boolean** — `valid`, `expired`,
  `static`, or `unknown`, each with the right action (`use`, `renew`, `none`), so
  a static key is never nagged to "renew" and an expired SSO session is obvious.
- **User-owned organization that survives discovery** — your labels and
  collections are stored separately from discovered inventory, so `sync` never
  erases them.
- **Cache first** — a local JSON inventory makes access easy to inspect and
  navigate. It is **not** a secret vault: the cache is deliberately not a general
  secret store.
- **Optional CLI management** — managing third-party provider CLIs may come later,
  but it is not the default model.
- **Operator-focused UX** — built to answer "what clusters do I have?", "which
  credential is expired?", and "what should I use next?" quickly.

## Core concepts

The CLI is built around a stable domain model:

- **Provider** — source of access such as `azure`, `aws`, `gcp`, or `kubeconfig`.
- **AccessSource** — a concrete source of access data (an Azure CLI profile, an
  AWS profile, a kubeconfig file).
- **Credential** — a usable identity inside a provider.
- **Scope** — an administrative or logical boundary (an Azure subscription, an AWS
  account/profile scope).
- **Target** — a selectable Kubernetes destination (a managed cluster or a
  kubeconfig context).
- **Labels** — key/value metadata used to organize targets.
- **Collections** — saved logical views over targets.

## Labels and collections

Kubernetes labels inspired the organization layer. Targets carry **system labels**
(discovered or derived by the tool) and **user labels** (defined by you).
Collections are not static folders — they are saved **views** over targets,
primarily driven by labels and selectors, with optional static members.

Because a collection is a **live query over labels**, you can create it first and
tag clusters into it later — order does not matter:

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
  `production` with no extra step; the collection re-resolves from current labels
  every time.
- **Survives discovery** — user labels are never wiped by `sync`, so collections
  keep matching across resyncs.
- **Cross-cloud** — one selector (`env=prod`) spans Azure, AWS, GCP, and
  kubeconfig at once.
- **Selectors** accept exact matches (`env=prod`), comma-joined or repeated
  `--selector`, and `in` lists (`"region in [westeurope, eu-central-1]"`). You can
  also select on a target's structured attributes by bare key: `region`,
  `platform`, `provider`, `health`, `kind`.
- **Static members** — add one-offs that don't fit a selector with
  `--static <target-id>` (unioned with the selector matches).
- **Manage labels** — `target label list <ref>` to see them,
  `target label remove <ref> <key>` to drop one. `kuberoutectl.io/` is a reserved
  system namespace; your labels are plain `key=value`.

A fuller walkthrough lives in the docs under
[Organizing: labels & collections](https://ymedlop.github.io/kuberoutectl/organizing/).

## Commands

Every inventory command supports `--output json` (`-o json`) for scripting.

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

## Installation

Pre-built binaries come in two flavors: **stable** releases cut from `vX.Y.Z`
tags, and a rolling **`development-snapshot`** pre-release rebuilt on every push
to `development`. Each build ships **Windows, Linux, and macOS in both `amd64` and
`arm64`**, plus `.deb`/`.rpm`/`.apk` Linux packages.

The [installation guide](https://ymedlop.github.io/kuberoutectl/installation/) has
the complete per-platform matrix, checksum verification, and troubleshooting. The
short version:

| Platform | Command |
|----------|---------|
| macOS | `brew install ymedlop/tap/kuberoutectl` |
| Debian/Ubuntu | `curl -1sLf '…/setup.deb.sh' \| sudo bash && sudo apt install kuberoutectl` |
| Fedora/RHEL/Alpine | `rpm -i` / `apk add --allow-untrusted` the release `.rpm`/`.apk` |
| Windows | `scoop bucket add ymedlop … && scoop install kuberoutectl` |
| Any (manual) | download the `…_<os>_<arch>` archive from [releases](https://github.com/ymedlop/kuberoutectl/releases), extract, put on `PATH` |

Not sure which architecture you need? Run `uname -m` — `x86_64` → `amd64`,
`aarch64`/`arm64` → `arm64`. See **[RELEASING.md](RELEASING.md)** for how releases
are produced and verified. Prefer to build it yourself? See
[Building from source](#building-from-source).

## Provider guides

Step-by-step manuals for managing clusters and credentials on each cloud:

- [Azure (AKS)](https://ymedlop.github.io/kuberoutectl/guides/azure/)
- [AWS (EKS)](https://ymedlop.github.io/kuberoutectl/guides/aws/) — including corporate IAM Identity Center / Entra sign-in
- [GCP (GKE)](https://ymedlop.github.io/kuberoutectl/guides/gcp/)
- [kubeconfig](https://ymedlop.github.io/kuberoutectl/guides/kubeconfig/) — self-hosted / local / handed-to-you contexts

See [the guides index](https://ymedlop.github.io/kuberoutectl/guides/) for the
shared model and the credential-health spectrum. The full docs, with search and
navigation, are published at
**[ymedlop.github.io/kuberoutectl](https://ymedlop.github.io/kuberoutectl/)**.

**Driving `kuberoutectl` from an AI assistant?** See the companion
[kuberoutectl-skills](https://github.com/ymedlop/kuberoutectl-skills) repo —
vendor-neutral operator skills for discovery, routing, and organizing.

## Building from source

Requires Go (see the `go` directive in `go.mod`). Common tasks are wrapped in the
`Makefile`:

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

The repository uses `main` for stable code and `development` for active
integration. The `snapshot-release` GitHub Actions workflow builds cross-platform
binaries with GoReleaser on every push to `development` — Windows, Linux, and
macOS, each in `amd64` and `arm64` — and replaces a single `development-snapshot`
pre-release. This makes it easy to develop on a personal machine and validate
builds on a more restricted work environment without promoting every test build to
a formal release.

**Regenerating the demo:** the README GIF is built from the committed provider
fixtures (no real cloud, no secrets) — run `make demo` (needs
[`asciinema`](https://asciinema.org) + [`agg`](https://github.com/asciinema/agg)).
The command flow lives in `scripts/demo.sh`. `make verify-readme` (run in CI)
asserts every command shown in the README and the demo still exists in the CLI,
so they can't silently drift after a rename.

## Roadmap

Post-1.0 work — additive, and it does not change the core workflow:

- managed `kubectl` runtime with version compatibility + selection ([#37](https://github.com/ymedlop/kuberoutectl/issues/37)–[#42](https://github.com/ymedlop/kuberoutectl/issues/42))
- an MCP server for `kuberoutectl` ([#44](https://github.com/ymedlop/kuberoutectl/issues/44))
- richer health checks and improved collection selectors

## Status

**1.0.0 is the first stable public release — a stability milestone, not a feature
milestone.** The core discover → organize → route workflow is complete across the
Azure, AWS, GCP, and kubeconfig providers, with a provider-agnostic core, a JSON
local cache, user labels and collections that survive resync, credential-health
awareness, and cross-platform package distribution (Homebrew, Scoop, deb/rpm/apk).
The command surface is not expected to change in breaking ways. See
[CHANGELOG.md](CHANGELOG.md) for the full 1.0.0 summary and
[RELEASING.md](RELEASING.md) for the release process. `TODO.md` is the historical
milestone-1 tracker, kept for reference.

The architecture is shaped around real operator workflows first, not around
generic abstractions for their own sake.

## Acknowledgements

[![Hosted By: Cloudsmith](https://img.shields.io/badge/OSS%20hosting%20by-cloudsmith-blue?logo=cloudsmith&style=flat-square)](https://cloudsmith.com)

Package repository hosting for the apt distribution is graciously provided by
[Cloudsmith](https://cloudsmith.com), which offers free package hosting for
open-source projects.

## License

[Apache License 2.0](LICENSE).
