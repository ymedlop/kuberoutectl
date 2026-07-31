# Plan: Release v1.1.0 — MCP server + provider diagnostics

**Spec**: `.planning/specs/release-1-1-0.md`
**Epic**: none
**Created**: 2026-07-26
**Status**: shipped in #102

---

## Stack note — why this plan has no layered architecture

Stack detected: **Go CLI** (`go.mod`, `cmd/kuberoutectl`, `internal/…`). No
`build.gradle.kts`, no `package.json` at the repo root — the Kotlin/Next.js
branches of the plan template do not apply.

More importantly: **this release ships zero new application code.** Every Go
change is already merged on `development` (#98–#101). The Controller → Manager →
Repository breakdown would be theatre here. The real architecture of this plan is
the **release pipeline**, so that is what is modelled below.

Confirmed by inspection:

- `internal/buildinfo` defaults to `Version = "dev"` and is overwritten by
  `-ldflags -X` from the git tag → **no source file carries the version number**,
  nothing to bump in code.
- No version-pinned install command exists in `docs/` (grep for `v1.0.0` /
  `VERSION=` returns nothing outside `RELEASING.md` prose and the README status
  paragraph).

So the change surface is exactly two Markdown files, plus a git/CI choreography.

---

## Components

| Component | Type | Purpose |
|-----------|------|---------|
| `CHANGELOG.md` | Release artifact | Hand-maintained Keep a Changelog record; GoReleaser changelog generation is disabled, so this is the only human-readable record |
| `README.md` | Release artifact | Roadmap + Status must stop claiming MCP is planned and 1.0.0 is current |
| `promote-to-main` branch | Git choreography | Carries development's content onto a base of `main` so the squash-merge ancestry trap is bypassed |
| `v1.1.0` tag | Release trigger | Fires `release.yml`; the only input GoReleaser needs for the version |
| `release.yml` | CI pipeline | `make check` → build all OS/arch → draft GitHub release + cask/scoop/Cloudsmith push |
| `ci.yml` | Verification gate | Runs `make check` **and** `scripts/e2e.sh`; triggers on `pull_request` and `push: [main]` |
| `pages` workflow | Post-release | Rebuilds the docs site from `main` after the promote |

## Files to change

| File | What changes | Why (spec ref) |
|------|--------------|----------------|
| `CHANGELOG.md` | Add `## [1.1.0] — 2026-07-26` section (Added / Fixed / Changed); backfill the `[1.0.0]` date to `2026-07-18`; add the `[1.1.0]` link ref at the bottom | Must-have #2 |
| `README.md` | Remove the MCP bullet from **Roadmap**; rewrite the **Status** paragraph so it describes 1.1.0 as current | Must-have #3 |

**No new files.** No Go, YAML, or script changes.

---

## Phases and tasks

### Phase 0 — Pre-flight (verification only, no edits)

Runs on `origin/development`. Nothing here mutates the repo; it decides whether
the release is even legal to cut.

| # | Task | Files | What to verify |
|---|------|-------|----------------|
| 0.1 | Confirm the divergence shape before touching anything | — | `git merge-base --is-ancestor origin/main origin/development` → expect **diverged**. This is the normal state and the reason for the recipe; a *clean* result means someone changed the merge strategy — stop and re-read `RELEASING.md` |
| 0.2 | Run the local verification ladder on `development` | — | `go test ./...`, `make check`, `bash scripts/e2e.sh` all green |
| 0.3 | Confirm the last `development` commit's snapshot build is green | — | `snapshot-release.yml` succeeded for `0d8cbfe`; a red snapshot means the release build will fail too |

**Gate: all three pass, or the release stops here.**

### Phase 1 — Release notes (depends on Phase 0)

Branch `chore/release-1-1-0-notes` off `origin/development`, PR into
`development`. Edits land on `development` **first**, not on `main` — otherwise
the promote in Phase 2 would carry different content than `development` has, and
`git diff --stat origin/development` (the promote's correctness check) could
never be empty.

| # | Task | Files |
|---|------|-------|
| 1.1 | Write the `[1.1.0]` CHANGELOG section — **Added**: `kuberoutectl mcp` stdio server + `--read-only` (#98, closes #44), `--verbose` provider CLI tracing (#99), generated command reference on the docs site (#94). **Fixed**: AWS `sso_session` profiles → `expired` + renew instead of `unknown` (#101), AWS expired-token diagnostic (#99). **Changed**: fixed-name macOS snapshot archives for the rolling brew cask (#100) | `CHANGELOG.md` |
| 1.2 | Backfill the `[1.0.0]` date (`2026-07-18`, from `git log -1 --format=%ci v1.0.0`), remove the "date is set when the tag is cut" placeholder, add the `[1.1.0]` link ref | `CHANGELOG.md` |
| 1.3 | README: drop `- an MCP server for kuberoutectl (#44)` from **Roadmap**; update **Status** (currently opens *"1.0.0 is the first stable public release"*) to name 1.1.0 as current while keeping the stability-milestone framing for 1.0.0 | `README.md` |
| 1.4 | Run `make check` — the CLI root test asserts every command shown in the README still exists, so a README edit can genuinely break tests | — |

Tasks 1.1 and 1.2 touch the same file → **one commit**. 1.3 is a separate commit.
One PR, two commits.

### Phase 2 — Promote `development` → `main` (depends on Phase 1 merged)

Follows `RELEASING.md` §"Promoting development to main" verbatim. **Not** a plain
`development → main` PR.

| # | Task | Files |
|---|------|-------|
| 2.1 | `git fetch origin main development`; `git switch -c promote-to-main origin/main`; `git merge origin/development` | — |
| 2.2 | Resolve every conflict in **development's** favour (`git checkout --theirs <file>` + `git add`), then commit. Expect conflicts on `README.md` and `RELEASING.md` — the files both branches always touch | conflicted files |
| 2.3 | **Correctness gate**: `git diff --stat origin/development` must print **nothing**. Non-empty output = development content was lost in the merge; fix before opening the PR | — |
| 2.4 | Open PR `promote-to-main` → `main`. If `gh pr edit` is needed to change the base, use the REST API instead (`gh api -X PATCH repos/ymedlop/kuberoutectl/pulls/<n> -f base=main`) — `gh pr edit` fails in this repo with a Projects-classic GraphQL error | — |
| 2.5 | Wait for `ci.yml` green **on the PR**, then merge | — |
| 2.6 | Wait for `ci.yml` green **on `main` after merge** (it triggers on `push: [main]`). This is the run that gates the tag | — |

### Phase 3 — Reproducibility check (parallel with Phase 2; blocks Phase 4)

| # | Task | Files |
|---|------|-------|
| 3.1 | `make snapshot`, then copy `dist/checksums.txt` aside — the target runs `goreleaser release --snapshot --clean`, and `--clean` wipes `dist/` at the start of the next run | — |
| 3.2 | `make snapshot` again; `diff` the two `checksums.txt`. Identical = pass. A difference in the `.deb`/`.rpm` lines specifically points at `SOURCE_DATE_EPOCH`, not at a source change | — |

**Cannot run in this sandbox** — GoReleaser is not installed and the network is
restricted. Must run on the maintainer's machine. If it is skipped, that must be
stated in the release notes, not silently omitted.

### Phase 4 — Tag and draft (depends on Phase 2.6 + Phase 3)

| # | Task | Files |
|---|------|-------|
| 4.1 | `git switch main && git pull`, verify HEAD is the green commit from 2.6 | — |
| 4.2 | `git tag v1.1.0 && git push origin v1.1.0` | — |
| 4.3 | Watch `release.yml`: `make check` → build → **draft** release + `checksums.txt`. `prerelease: auto` + no suffix on `v1.1.0` = full release, still a draft | — |

### Phase 5 — Publish and verify (human maintainer)

| # | Task | Files |
|---|------|-------|
| 5.1 | Review the draft on GitHub: all OS/arch archives present, `checksums.txt` present, notes accurate. Add the MCP-not-client-tested caveat to the body | — |
| 5.2 | **Click Publish** — human only. This is irreversible for package consumers: publishing is what updates the Homebrew tap and Scoop bucket | — |
| 5.3 | Post-release smoke: `brew install ymedlop/tap/kuberoutectl`; `kuberoutectl version` → `1.1.0`; `kuberoutectl mcp --help` → exit 0 | — |
| 5.4 | Confirm the `pages` workflow rebuilt the docs site from `main` and `/mcp/` renders | — |

---

## Parallel vs sequential

| Parallel group | Tasks | Why |
|----------------|-------|-----|
| Group A | 1.1+1.2 (CHANGELOG) and 1.3 (README) | Different files, no shared state |
| Group B | Phase 3 (reproducibility) and Phase 2 (promote) | The snapshot build reads `development`'s content, which Phase 2 only moves, never modifies |

| Sequential | Depends on | Why |
|------------|-----------|-----|
| Phase 1 | Phase 0 | No point writing notes for a release that cannot pass its own tests |
| Phase 2 | Phase 1 merged | The promote must carry the final notes; otherwise 2.3's empty-diff gate is meaningless |
| 2.3 | 2.2 | The gate only means something after every conflict is resolved |
| 2.6 | 2.5 | `ci.yml` on `main` only fires on push, i.e. after the merge |
| Phase 4 | 2.6 **and** Phase 3 | `release.yml` runs only `make check` — it never re-runs `scripts/e2e.sh`, so e2e green on `main` is the tag's real gate |
| 5.2 | 5.1 | Publishing is irreversible for the tap/bucket |

---

## Testing plan

No unit tests are added — this release introduces no code. The test plan is the
existing suite plus release-specific gates, each tied to a spec item.

| Layer | Test | Spec ref |
|-------|------|----------|
| Unit | `go test ./...` on `development` (Phase 0.2) and again on the promote branch | Must-have #5 |
| Pre-commit gate | `make check` (fmt + vet + test) after the README edit — the root command test asserts README commands still exist, so this covers the docs-drift edge case | Edge case: docs drift |
| Integration | `bash scripts/e2e.sh` — 4-provider operator flow with fake `az`/`aws`/`gcloud`/`kubectl`. Must be green **before** the tag, because `release.yml` never runs it | Edge case #5 |
| CI | `ci.yml` green on the promote PR and on `main` post-merge | Must-have #5 |
| Build reproducibility | Two `make snapshot` runs → identical `dist/checksums.txt` (Phase 3) | Must-have #4, edge case #3 |
| Promote correctness | `git diff --stat origin/development` empty on the promote branch (2.3) | Edge case #1 |
| Snapshot template | `snapshot.version_template` still uses the fixed `0.0.0-snapshot-{{ .ShortCommit }}` prefix — `{{ incpatch .Version }}` breaks against the non-semver `development-snapshot` tag | Edge case #2 |
| Release artifact | Downloaded binary reports `1.1.0` + tagged commit; `kuberoutectl mcp --help` exits 0 (5.3) | Goal assertions |
| Changelog integrity | `[1.1.0]` and `[1.0.0]` both have real dates and resolvable link refs | Must-have #2 |

### Known coverage gap (state, don't hide)

**Corrected 2026-07-26.** `scripts/e2e.sh:238-253` *does* drive the shipped
binary over stdio (`initialize` → `notifications/initialized` → `tools/list`)
and asserts `--read-only` hides write tools; typed `tools/call` round-trips are
covered in-memory. The remaining gap is narrower: **no third-party MCP client**
is exercised and no tool runs against a real cloud. Scope the release-note
caveat to that (spec edge case #7).

---

## Risks and their triggers

| Risk | Trigger to watch | Response |
|------|------------------|----------|
| Homebrew cask push fails | `release.yml` red *after* the tag exists | Re-run the job; do not re-tag. Cause is usually an expired `HOMEBREW_TAP_GITHUB_TOKEN` (a PAT, not `GITHUB_TOKEN`) |
| Tag on a red commit | `ci.yml` on `main` not green at 2.6 | Do not tag. Fix on `development`, re-promote |
| Promote loses content | 2.3 prints a non-empty diff | Redo the merge resolution toward development |
| deb/rpm non-reproducible | 3.2 diff differs only on package lines | `SOURCE_DATE_EPOCH` missing from the environment — check `Makefile`/workflow, not the source |

---

## Gate 2 — checklist

**Architecture**
- [x] Follows existing project patterns — the release choreography is `RELEASING.md`'s documented recipe, not an invention
- [x] Layering respected — Phase N never depends on a later phase; the one cross-cutting job (Phase 3) is explicitly marked parallel-safe
- [x] Files in the right place — both edited files are repo-root release artifacts, matching where they already live

**Task breakdown**
- [x] All files to change listed (2: `CHANGELOG.md`, `README.md`)
- [x] All new files listed (none — stated explicitly)
- [x] Each task small — max 1 file per task, 2 commits in the only code PR
- [x] Dependencies explicit (sequential table)
- [x] Parallel vs sequential marked

**Testing**
- [x] Unit tests planned (`go test ./...`)
- [x] Business-logic / integration tests planned (`scripts/e2e.sh`)
- [x] CI gate planned (`ci.yml` on PR + on `main`)
- [x] UI tests — n/a (no UI); docs-site render check covers the closest equivalent
- [x] Every spec edge case mapped to a test row or a risk row (1→promote gate, 2→snapshot template, 3→reproducibility, 4→cask risk, 5→e2e before tag, 6→human-only publish, 7→stated caveat)

**Gate 2: PASSED**
