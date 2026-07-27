# Changelog

All notable changes to `kuberoutectl` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is maintained by hand (GoReleaser's changelog generation is disabled).

## [Unreleased]

### Fixed
- **A malformed provider response no longer looks like an empty account.** When
  `aws eks list-clusters` / `describe-cluster` or `gcloud container clusters list`
  **succeeded** but returned output that could not be parsed, the cluster (or the
  whole profile/project) was skipped in complete silence — and because the
  command itself succeeded, `--verbose` showed nothing wrong either. An output
  format change from a provider CLI therefore read as "you have no clusters".
  These cases are still skipped rather than failing the sync, since one bad
  cluster must not sink the whole inventory, but each now prints a diagnostic
  naming what could not be read and flagging a possible CLI format change. The
  wording is deliberately distinct from an access denial: one is routine in a
  fleet with uneven permissions, the other is worth investigating.
- **A cluster reachable by several AWS profiles was listed twice, and one copy
  was unreachable.** An EKS ARN identifies the account, not the profile, so two
  profiles authenticating into the same account produced two targets with an
  **identical id**. Aliases are disambiguated by hashing the id, so both rows
  also got the same alias, and reference resolution returns the first match —
  leaving the second silently unreachable by anything the CLI prints. Such
  clusters now fold into one target that records every profile reaching it.
  ([#109](https://github.com/ymedlop/kuberoutectl/pull/109))

### Added
- **`target use <ref> --profile <name>`** — choose which credential to go in
  through when several reach a target, instead of always taking the default. The
  choice is remembered, so a later bare `target use` reuses it, and `current`
  reports the profile your kubeconfig was actually written with. A profile that
  cannot reach the target is rejected before anything runs, naming the ones that
  would work. The same argument is available on the MCP `use_target` tool, so an
  AI client is not restricted to the default.
- **`target list` gains a `PROFILES` column** and `target inspect` a per-profile
  health breakdown, both shown only when a target actually has a choice.
- **Per-cluster access denials are reported during `sync aws`.** `eks:ListClusters`
  cannot be scoped below account/region while `eks:DescribeCluster` is evaluated
  per cluster, so a profile that lists everything may still be denied on
  individual ones. That used to be skipped silently; naming it turns an
  undocumented access map into ordinary sync output.
- New `kuberoutectl.io/credential` system label on every provider's targets,
  naming the target's default credential so it can be selected on.

### Known limitation
- Profile reachability is resolved at the **IAM** layer (`eks:DescribeCluster`).
  Operating inside a cluster additionally requires an EKS **access entry**, which
  is a Kubernetes-side mechanism kuberoutectl does not read. Where profiles
  differ only at that layer, the chosen default is a guess — which is why it is
  labelled `(default — pass --profile to pick another)` rather than presented as
  a decision.

### Added
- **CI now verifies reproducible builds.** A `reproducible-build` workflow
  builds the full snapshot twice and diffs `dist/checksums.txt`, running on
  demand (before cutting a tag), on pull requests that touch the build
  configuration, and weekly — a runner image bumping its Go toolchain can break
  reproducibility with no repo change at all. Until now this was a manual
  checklist item, which is why it was skipped for 1.1.0 and that release's notes
  had to record the guarantee as unproven for that tag.

## [1.1.1] — 2026-07-26

A documentation-accuracy patch. No runtime behaviour changed: every command,
flag, and MCP tool works exactly as it did in 1.1.0 — what changed is that the
help text now tells the truth about which providers you can pass.

### Fixed
- **`target list --provider` help advertised only two of the four providers.**
  `kuberoutectl target list --help` said `filter by provider (azure|aws)` in both
  the flag usage and the command's long help, while `gcp` and `kubeconfig` were
  registered and the filter accepted them (it passes the value straight through —
  there was never a whitelist). Only the text was wrong, so nothing failed when
  it drifted. The list is now derived from the provider registry rather than
  written by hand, so it cannot go stale again, and a test asserts every
  registered provider appears in the help of both `target list` and
  `credential list`.

### Added
- A regression guard for the MCP tools' `provider` **jsonschema descriptions**,
  which carry the same hand-written provider list. They were already accurate —
  nothing was broken — but nothing tied them to the registry, so they could
  drift exactly as the CLI help did. Struct tags are compile-time constants and
  cannot be derived, so a test now reads them by reflection and requires the
  listed ids to match the registry exactly. No schema or tool behaviour changed.

### Verified
- **Reproducible builds re-confirmed** for this tag: two consecutive
  `make snapshot` runs produced an identical `dist/checksums.txt`, covering the
  archives and the `.deb` / `.rpm` packages. This was skipped for 1.1.0; the
  build configuration (`mod_timestamp` + `SOURCE_DATE_EPOCH`) is unchanged
  between the two tags.
- Unit tests, `make check`, and `scripts/e2e.sh` — the latter drives the shipped
  binary through a real MCP stdio handshake.

## [1.1.0] — 2026-07-26

The first feature release after 1.0.0. Everything here is **additive** — no
existing command changes its shape, so upgrading is a drop-in replacement.

### Added
- **`kuberoutectl mcp`** — an optional [Model Context Protocol](https://modelcontextprotocol.io)
  server over stdio, so an MCP-capable AI client can drive the safe core of the
  inventory (list targets, inspect credential health, manage collections). The
  server is opt-in: nothing runs unless you start it. `--read-only` exposes the
  inspection tools only, with no sync / use / collection writes.
  ([#98](https://github.com/ymedlop/kuberoutectl/pull/98), closes
  [#44](https://github.com/ymedlop/kuberoutectl/issues/44))
- **`--verbose` / `-v`** (global) — traces every external CLI invocation, its
  exit code, and its stderr on failure. Turns "discovery found nothing" from a
  guess into something you can read.
  ([#99](https://github.com/ymedlop/kuberoutectl/pull/99))
- A generated **command reference** on the docs site, produced from the Cobra
  command tree so it cannot drift from the binary.
  ([#94](https://github.com/ymedlop/kuberoutectl/pull/94))

### Fixed
- **AWS: modern `sso_session` profiles are now recognised as SSO.** A profile
  using the `sso_session` form (rather than the legacy inline `sso_start_url`)
  was classified as `unknown` health with no suggested action; it is now
  reported as `expired` with a renew action, so `aws sso login` is actually
  offered. ([#101](https://github.com/ymedlop/kuberoutectl/pull/101))
- **AWS: expired-token diagnostic.** A failed identity check now says the token
  has likely expired and names the exact `aws sso login --profile <name>` to
  run, instead of silently yielding zero clusters.
  ([#99](https://github.com/ymedlop/kuberoutectl/pull/99))

### Changed
- Snapshot builds publish fixed-name macOS archives, so the rolling Homebrew
  cask for `development` keeps working across snapshots. Stable releases are
  unaffected. ([#100](https://github.com/ymedlop/kuberoutectl/pull/100))

### Verified / not verified
- Verified: unit tests, `make check`, and `scripts/e2e.sh` — the latter drives
  the **shipped binary** through a real MCP stdio handshake (`initialize` →
  `notifications/initialized` → `tools/list`) and asserts `--read-only`
  withholds the write tools.
- Not verified: no **third-party MCP client** (Claude Desktop and friends) is
  exercised by the test suite, and no provider tool is run against a real cloud
  account — the provider flows are fixture-driven with fake
  `az` / `aws` / `gcloud` / `kubectl`.

## [1.0.0] — 2026-07-18

The first **stable** public release. 1.0.0 is a **stability milestone**, not a
feature milestone: the core discover → organize → route workflow is complete and
the command surface is not expected to change in breaking ways.

### Discover
- Provider-agnostic core with a provider registry and capability flags.
- **Azure (AKS)**, **AWS (EKS)**, **GCP (GKE)**, and **kubeconfig** discovery.
- Kubeconfig contexts that duplicate a natively-discovered cluster (matched by
  API-server endpoint) are suppressed, so a cluster isn't inventoried twice.
- Normalized Kubernetes server version persisted per target (`unknown` when a
  provider has no source, e.g. kubeconfig).

### Organize
- User **labels** and selector-driven **collections** over targets; both survive
  every discovery resync (JSON `cache/` vs `state/` separation).
- Target **visibility** (`hide` / `unhide`, persistent) and ephemeral cache
  curation (`delete` / `clear`).
- Credential **health** awareness (valid / expiring / expired / static / unknown)
  with suggested actions; static credentials are never coerced into a renew flow.

### Route
- `target use` writes kubeconfig and points `kubectl` at the selected cluster;
  `current` reports what you're pointed at and cache freshness.
- Deterministic, `-o json`-capable inventory output.

### Distribute
- Cross-platform release artifacts for Windows, Linux, and macOS (amd64 + arm64):
  `.tar.gz` / `.zip` archives plus `checksums.txt`.
- Package-manager install paths: **Homebrew** (cask), **Scoop**, and Linux
  **`.deb` / `.rpm` / `.apk`** packages.
- **Reproducible builds** — the same commit produces byte-identical artifacts.
- Documented, repeatable release automation (draft GitHub release on a `vX.Y.Z`
  tag) with the packaging config validated in CI.

### Docs
- Provider guides, an installation guide with a troubleshooting section, and a
  labels & collections guide, published to GitHub Pages.

[1.1.1]: https://github.com/ymedlop/kuberoutectl/releases/tag/v1.1.1
[1.1.0]: https://github.com/ymedlop/kuberoutectl/releases/tag/v1.1.0
[1.0.0]: https://github.com/ymedlop/kuberoutectl/releases/tag/v1.0.0
