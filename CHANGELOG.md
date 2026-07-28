# Changelog

All notable changes to `kuberoutectl` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is maintained by hand (GoReleaser's changelog generation is disabled).

## [Unreleased]

### Fixed
- **A malformed provider response no longer looks like an empty account.** When
  `az aks list`, `aws eks list-clusters` / `describe-cluster`, or
  `gcloud container clusters list` **succeeded** but returned output that could
  not be parsed, the cluster (or the
  whole subscription/profile/project) was skipped in complete silence — and because the
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
- **`target list` gains a `VERSION` column**, showing the Kubernetes server
  version discovery already records. It was previously reachable only through
  `target inspect`, one cluster at a time, despite being among the first things
  you look at when choosing one. Nothing new is fetched — the value is already in
  the snapshot. A target cached before versions were tracked renders `unknown`
  rather than blank, since an empty cell in a table reads as a value.
- **`target list` gains a `PROFILES` column** and `target inspect` a per-profile
  health breakdown, both shown only when a target actually has a choice.
- **Per-cluster access denials are reported during `sync aws`.** `eks:ListClusters`
  cannot be scoped below account/region while `eks:DescribeCluster` is evaluated
  per cluster, so a profile that lists everything may still be denied on
  individual ones. That used to be skipped silently; naming it turns an
  undocumented access map into ordinary sync output.
- New `kuberoutectl.io/credential` system label on every provider's targets,
  naming the target's default credential so it can be selected on.

- **`sync aws` now reads EKS access entries, so the default profile is picked on
  whether the cluster admits it — not just on whether it can authenticate.**
  Reachability was resolved at the **IAM** layer (`eks:DescribeCluster`), but
  operating inside a cluster additionally requires an EKS **access entry**, a
  Kubernetes-side authorization layer. Where profiles differed only there, a
  profile could describe a cluster, be chosen as the default, activate cleanly,
  and still get `Forbidden` from `kubectl` — with nothing in the inventory
  hinting at it. For clusters more than one profile reaches, discovery now makes
  one `list-access-entries` call per cluster and prefers an admitted profile
  **even over a healthier one**: an expired session is one `aws sso login` away,
  while a missing access entry cannot be fixed from this CLI at all.

  What can be concluded depends on the cluster's `authenticationMode`, and only
  a *negative* answer does: under `API` the list is authoritative both ways;
  under `API_AND_CONFIG_MAP` and `CONFIG_MAP` a listed profile is still
  confirmed but an absent one is **unknown**, because `aws-auth` may grant it and
  kuberoutectl deliberately does not read `aws-auth` (that would need working
  `kubectl` access to the cluster being asked about). Clusters created through
  the API, the SDKs or CloudFormation default to `CONFIG_MAP`, so `unknown` is
  the normal answer rather than a failure, and it is never rendered as "no".

  `target inspect` gains an `Access check` line and a per-profile
  verdict. `target use` with a profile the cluster is **confirmed** to refuse
  warns on stderr, names one that would work, and proceeds anyway — the verdict
  is from the last sync and may be stale, and entering a cluster to diagnose
  exactly this is legitimate. A profile whose verdict is `unknown` produces no
  warning at all. The MCP `use_target` tool returns the same caution as an
  `access_warning` field, from the same service call, so the two surfaces cannot
  drift into disagreeing about when to warn.

  **Every cluster whose authentication mode permits a conclusion is checked**,
  one call each, including those a single profile reaches. An earlier revision of
  this work skipped those, reasoning that with one way in there is nothing to
  choose — true, but there is still something to *know*: whether that one way in
  will be refused. Validated against a real fleet, that bound turned out to skip
  nearly everything, leaving `unknown` almost everywhere. `CONFIG_MAP` clusters
  still cost nothing, since the mode arrives with `describe-cluster`, and `sync`
  now reports how many clusters it listed entries for so the cost is visible.
  Without `eks:ListAccessEntries` the verdict is `unavailable`, every profile
  reads `unknown`, and `sync` names the missing permission.

- **`target use --refresh` and `target inspect --refresh`** — re-establish
  operability against the cluster instead of trusting the last sync, one API call
  for the cluster you named. The case it exists for is narrow and common: *you
  have just been granted access and want to know whether it landed*, without
  resyncing the fleet. Under the flag the answer is reported in **both**
  directions — a confirmed admission is as much of an answer as a refusal — and a
  check that cannot conclude says why.

  **Nothing checks without it.** `sync` already covers every cluster, so a live
  check buys freshness rather than coverage and should cost a call only when
  asked for; `target list` never checks at all, since one call per row on every
  display is the cost that is never worth paying. A failed check never blocks the
  activation: refusing to write a kubeconfig because the EKS API hiccuped would
  lock you out of a cluster at exactly the moment you are diagnosing it. The MCP
  `use_target` and `get_target` tools take the same `refresh` argument with the
  same default, so an agent and a human are never told different things about the
  same cluster, and `use_target` returns an `access_verdict` alongside the
  existing `access_warning`.

- **`target list -l operable=true`** — the verdict is queryable as a selector,
  alongside `region`, `platform` and `health`, so "which of these can I actually
  operate?" is one question rather than a column to scan. Three values —
  `true`, `false`, `unknown` — with `unknown` always present rather than absent,
  since in most fleets it is the largest set and the one worth enumerating. It
  composes with the rest of the selector grammar and with collections.

- **`doctor` now tells you when your binary is out of date**, and
  `version --check-update` asks the same question deliberately. Someone on an old
  release had no way to learn a newer one existed short of visiting the repo —
  the cost of which is not a missing feature but **debugging a bug that is
  already fixed**, which is exactly what sends people to `doctor` in the first
  place. The verdict is an ordinary check row, so it inherits `doctor`'s
  rendering and `-o json` shape, and it is appended rather than inserted so a
  consumer indexing that array sees an added element and not shifted ones.

  **No other command makes a network request.** Not `sync`, `target`,
  `collection`, `current` or `mcp` — and a test enforces that only the CLI layer
  can import the code that would. There is no ambient check and nothing is
  cached: an unsolicited outbound call on every invocation is a sentence a tool
  that handles cloud credentials has to defend in every security review, and
  confining it to two commands whose names say "diagnostic" is a much easier one.
  The request itself is an unauthenticated `GET` that transmits nothing — no
  version, no identifier, no usage data. Set `KUBEROUTECTL_NO_UPDATE_CHECK` to
  any value to suppress the request as well as the output.

  A build that is not a stable release (`dev`, a snapshot) never checks, and a
  pre-release upstream is never offered to someone on a stable version. An
  unreachable or rate-limited API — the likely case behind a corporate NAT
  sharing one address across 60 requests an hour — is reported as "could not
  check" and **never** as "you are up to date": a check that vanishes when it
  fails is indistinguishable from one that was never wired up.

- **CI now verifies reproducible builds.** A `reproducible-build` workflow
  builds the full snapshot twice and diffs `dist/checksums.txt`, running on
  demand (before cutting a tag), on pull requests that touch the build
  configuration, and weekly — a runner image bumping its Go toolchain can break
  reproducibility with no repo change at all. Until now this was a manual
  checklist item, which is why it was skipped for 1.1.0 and that release's notes
  had to record the guarantee as unproven for that tag.

### Known limitation
- An access entry answers "are you admitted", not "may you do X" — a restrictive
  access policy still yields `Forbidden` on specific verbs, which only
  `kubectl auth can-i` can tell you. Two IAM roles with the same name under
  different paths also reduce to the same principal and match each other; if that
  shape exists in your account, the match is a false positive.

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
