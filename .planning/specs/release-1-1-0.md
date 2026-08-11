# Spec: Release v1.1.0 — MCP server + provider diagnostics

**Created**: 2026-07-26
**Status**: shipped in #102
**Author**: Yeray Medina López
**Epic**: none

---

## Problem

`main` is still at `v1.0.0` (tagged 2026-07-18), but `development` has
accumulated four shippable commits since then — including a whole new opt-in
surface, the `kuberoutectl mcp` server. Users installing through the stable
channels (Homebrew cask, Scoop, apt/deb/rpm/apk, GitHub release archives) cannot
get any of it: those channels only ever point at a stable `vX.Y.Z` tag. The
`development-snapshot` pre-release carries the code, but it is a rolling,
unversioned artifact that nobody should be asked to install.

Two of those commits are user-visible bug fixes (AWS `sso_session` profiles
reported as `unknown` instead of `expired`), so the gap is not only about
features — the released binary is actively wrong for a common AWS setup.

## Goal

`v1.1.0` is published as a GitHub release, cut from `main`, containing the MCP
server and the provider-diagnostics work, with all package channels
(Homebrew cask, Scoop, Cloudsmith apt/rpm) pointing at it.

Success is verifiable:

- `git describe --tags origin/main` → `v1.1.0`
- `brew install ymedlop/tap/kuberoutectl && kuberoutectl version` → `1.1.0`
- `kuberoutectl mcp --help` works in the released binary
- `CHANGELOG.md` has a `[1.1.0]` section with a real date, and `[1.0.0]` no
  longer says "date is set when the tag is cut"
- `README.md` no longer lists the MCP server under **Roadmap**

## User Stories

- **As an AWS user with a modern `sso_session` profile**, I want a released
  binary that reports my credential as *expired* with a renew action, so that
  I stop seeing `unknown` and having to guess whether re-login will help.
- **As someone wiring kuberoutectl into an AI client**, I want `kuberoutectl mcp`
  in a versioned, installable release, so that I can pin a version in my setup
  instead of depending on a rolling snapshot.
- **As a maintainer**, I want the promote → tag → publish path executed exactly
  as `RELEASING.md` documents, so that the squash-merge ancestry trap does not
  bite again on the next promote.

## Requirements

### Must have

1. **Promote `development` → `main`** using the branch-off-`main` recipe in
   `RELEASING.md` §"Promoting development to main". Not a plain
   `development → main` PR.
2. **CHANGELOG**: add a `## [1.1.0] — <date>` section (Keep a Changelog format,
   hand-maintained) covering:
   - **Added** — `kuberoutectl mcp` stdio MCP server, `--read-only` mode (#98,
     closes #44); `--verbose` provider CLI tracing (#99); generated command
     reference on the docs site (#94).
   - **Fixed** — AWS `sso_session` profiles detected as SSO → `expired` +
     renew action instead of `unknown` (#101); AWS expired-token diagnostic (#99).
   - **Changed** — fixed-name macOS snapshot archives for a rolling brew cask
     (#100).
   - Backfill the `[1.0.0]` date (`2026-07-18`) and add the `[1.1.0]` link ref.
3. **README roadmap cleanup**: remove
   `- an MCP server for kuberoutectl (#44)` from the **Roadmap** list; the
   README's own "MCP server (optional)" section already documents it as shipped.
   Update the **Status** section if it asserts 1.0.0 is the current release.
4. **Verify reproducible builds**: `make snapshot` twice, `dist/checksums.txt`
   byte-identical between runs. Blocking — it is a checklist item in
   `RELEASING.md` and `SOURCE_DATE_EPOCH` regressions are silent.
5. **Run the full verification ladder on the promote branch**: `go test ./...`,
   `make check`, `bash scripts/e2e.sh`. `release.yml` only runs `make check`, so
   e2e must pass *before* the tag.
6. **Tag `v1.1.0` on `main`** and push it, triggering `release.yml` → draft
   release with all OS/arch archives + `checksums.txt`.
7. **Maintainer publishes the draft** after reviewing artifacts. Publishing is a
   manual human step — an agent must not click Publish.

### Nice to have

- A short release-notes blurb in the draft body highlighting MCP, rather than
  relying only on GoReleaser's generated notes.
- Confirm the docs site rebuilds from `main` after the promote (the `pages`
  workflow) and that `/mcp/` renders.

### Out of scope

- Any change to the `kuberoutectl version` command itself — the "version" in
  this task is the release number, not the command.
- New features, new providers, or changes to the MCP tool surface. `v1.1.0`
  ships what is already on `development`, nothing more.
- Signing / SLSA provenance. Releases remain checksum-only (see `RELEASING.md`);
  cosign stays deferred.
- Holding any commit back from the release — everything on `development` ships.

## Data Model

Not applicable — no persisted-schema change in this release.

## API Changes

No changes introduced *by this spec*. The release publishes an already-merged
surface:

- New command `kuberoutectl mcp` (stdio MCP server, `--read-only` flag).
- New global flag `--verbose` (provider CLI tracing).

Both are additive; no existing command signature changes. This is why the bump
is **minor** (`1.0.0` → `1.1.0`) and not patch or major.

## UI Changes

None beyond CLI help text already merged. Docs site gains no new page — `docs/mcp.md`
and `docs/reference/kuberoutectl_mcp.md` already exist on `development`.

## Edge Cases

1. **Squash-merge ancestry trap.** `git merge-base --is-ancestor origin/main
   origin/development` will almost certainly report *diverged*. A plain
   `development → main` PR 3-way-merges against a stale base and conflicts on
   `README.md` / `RELEASING.md`. → Must use the branch-off-`main` recipe, and
   `git diff --stat origin/development` on the promote branch **must be empty**
   before opening the PR.
2. **Non-semver `development-snapshot` tag.** The rolling snapshot tag is not
   valid semver; GoReleaser template expressions like `{{ incpatch .Version }}`
   break against it. Verify the snapshot pipeline still uses the fixed
   `0.0.0-snapshot-{{ .ShortCommit }}` prefix before relying on `make snapshot`
   for the reproducibility check.
3. **`SOURCE_DATE_EPOCH` missing → deb/rpm differ on every build.**
   `mod_timestamp` does not cover the `nfpms:` pipe. If the two `make snapshot`
   runs differ, suspect this before assuming a real source change.
4. **Homebrew cask push fails on tag.** `homebrew_casks:` pushes to
   `ymedlop/homebrew-tap` using `HOMEBREW_TAP_GITHUB_TOKEN`, not the default
   `GITHUB_TOKEN`. An expired PAT fails the release job *after* the tag exists —
   recovery is re-running the workflow, not re-tagging.
5. **Tag pushed on a red commit.** `release.yml` runs only `make check`; e2e is
   never re-run. Tagging a commit whose `ci.yml` run on `main` has not gone green
   can ship something `scripts/e2e.sh` would have caught. Wait for green.
6. **Draft release auto-published by accident.** `release: draft: true,
   prerelease: auto`. `v1.1.0` (no pre-release suffix) lands as a **full**
   draft. Publishing is irreversible for package consumers — the tap and Scoop
   bucket update on publish.
7. **MCP verified over stdio, but not against a third-party client.**
   *(Corrected 2026-07-26 after inspecting `scripts/e2e.sh:238-253` — the
   original wording claimed no stdio coverage at all, which was wrong.)* The e2e
   flow does drive the **shipped binary** through a real JSON-RPC handshake
   (`initialize` → `notifications/initialized` → `tools/list`) and asserts
   `--read-only` hides the write tools; typed `tools/call` round-trips are
   covered by the in-memory Go test. What remains uncovered: no **third-party
   MCP client** (Claude Desktop, etc.) is exercised, and no tool runs against a
   real cloud. The release note caveat must be scoped to that, not to stdio.

## Testing Criteria

### Happy path

- `go test ./...` green on the promote branch.
- `make check` (fmt + vet + test) green.
- `bash scripts/e2e.sh` green — the 4-provider operator flow with fake
  `az`/`aws`/`gcloud`/`kubectl`.
- `ci.yml` green on `main` at the commit to be tagged.
- After publish: `kuberoutectl version` on a downloaded artifact reports
  `1.1.0` + the tagged commit; `kuberoutectl mcp --help` exits 0.

### Edge cases

- **Promote correctness**: `git diff --stat origin/development` on the promote
  branch is empty (no output). A non-empty diff means a conflict was resolved in
  `main`'s favour and development content was lost.
- **Reproducibility**: two consecutive `make snapshot` runs produce an identical
  `dist/checksums.txt` (`diff` reports nothing), covering the archive *and* the
  deb/rpm pipes.
- **Docs drift guard**: the README/demo command-existence test (`internal/cli`
  root test) still passes after the roadmap edit.
- **Changelog integrity**: `[1.1.0]` and `[1.0.0]` both carry real dates and
  resolvable link refs.

## Dependencies

- Green `ci.yml` on `main` after the promote PR merges.
- Repo secrets present and valid: `HOMEBREW_TAP_GITHUB_TOKEN` (tap push),
  Scoop bucket token, Cloudsmith credentials (apt/rpm).
- `ymedlop/homebrew-tap` and the Scoop bucket repos reachable.
- GoReleaser runs **only in CI** — it is not installed in the sandbox and the
  network is restricted, so `.goreleaser.yaml` changes cannot be validated
  locally. The reproducibility check via `make snapshot` therefore has to run
  somewhere GoReleaser exists (maintainer machine or CI), and this is a stated
  caveat, not a silently skipped step.
- Human maintainer to review and publish the draft release.

---

## Gate 1 — checklist

**Completeness**
- [x] Problem clearly stated (stable channels stuck at 1.0.0 with a known AWS bug)
- [x] Goal specific and measurable (5 verifiable assertions)
- [x] User stories present (3)
- [x] Requirements split must / nice-to-have / out of scope
- [x] Out of scope section exists

**Data model** — n/a (no schema change)

**API design** — n/a for new surface; existing additive surface documented, and
it justifies the minor bump

**Quality**
- [x] Edge cases listed (7)
- [x] Happy-path testing criteria
- [x] Edge-case testing criteria
- [x] Dependencies listed

**Gate 1: PASSED**
