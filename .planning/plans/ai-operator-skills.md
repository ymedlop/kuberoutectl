# Plan: AI operator skills — standalone distributable repo

**Issue**: https://github.com/ymedlop/kuberoutectl/issues/43
**Spec**: none (planned from the issue)
**Epic**: none
**Created**: 2026-07-20
**Status**: shipped — repo https://github.com/ymedlop/kuberoutectl-skills (CI green vs v1.0.0); README pointer in kuberoutectl PR #72
**Stack**: A NEW standalone repo of vendor-neutral markdown skills + a one-line
pointer from this repo. No app code.

## Goal

A **standalone, distributable** repository of AI **operator** skills that teach
any AI assistant how to *use the shipped `kuberoutectl` CLI on behalf of an end
user*. Intended to be consumed independently of the CLI — cloned directly,
pointed at by external AI clients, or surfaced via a skills marketplace / the
future MCP server (#44).

## Decision (confirmed with user — reverses the earlier in-repo plan)

**Separate repo, not in-repo `skills/`.** The user's goal is standalone
distribution (external AI clients / marketplace), which is the "AI skills **repo**"
reading of the issue title. This is the deliberately higher-maintenance path; its
one real downside (drift + can't relatively reference the CLI docs) is mitigated
below rather than ignored.

- **Repo:** `ymedlop/kuberoutectl-skills` (public, Apache-2.0). *(Name to confirm
  at build; matches the `homebrew-tap`/`scoop-bucket` naming shape.)*
- **Docs references:** absolute URLs to the published site
  (`https://ymedlop.github.io/kuberoutectl/...`), not relative links — since the
  skills live in a different repo. Each reference notes the kuberoutectl version
  the skill was verified against.
- **Drift guard (the sustainability piece):** a CI job in the skills repo that
  installs a pinned `kuberoutectl` release and greps `--help` to confirm every
  referenced command/flag still exists — so the separate-repo drift the user
  accepted is *managed*, not silent.

## What the new repo contains

| Path (in `kuberoutectl-skills`) | Purpose |
|---|---|
| `README.md` | what this is (operator skills for kuberoutectl), who consumes it (any AI / MCP), the shared skill template + conventions, and the version it targets |
| `LICENSE` | Apache-2.0 (matches kuberoutectl) |
| `inventory-discovery/SKILL.md` | discovery workflow |
| `target-selection/SKILL.md` | select + route (incl. `hide`/`unhide`) |
| `label-and-collection-management/SKILL.md` | labels + collections |
| `credential-inspection/SKILL.md` | credential health + renew |
| `sync-and-verify/SKILL.md` | safe sync + verification |
| `release-and-status/SKILL.md` | `version` + `current` status |
| `.github/workflows/verify-commands.yml` | drift guard — installs kuberoutectl, asserts referenced commands exist |

## Design principles (from the ACs)

- One skill per user intent; maps to a real workflow with real commands.
- **Reference, don't duplicate** — link (absolute) to `docs/guides/`,
  `docs/organizing.md`, `docs/index.md`, `docs/installation.md`; never copy them.
- **Progressive disclosure** — "Use this when" + happy-path first; detail below.
- **Safety** — flag mutating vs read-only; destructive ones (`target delete`,
  `target clear`, `credential renew`) tell the AI to **confirm with the user**
  first (the CLI itself only prompts for `clear`). `sync` is safe/idempotent;
  `target use` writes kubeconfig; `hide`/`unhide` are safe + reversible.
- **Vendor-neutral** — no `Claude`/`Anthropic`, no Claude-Code frontmatter
  (`allowed_tools`); frontmatter (`name`/`description`) is generic index metadata,
  not runtime-parsed. Not a copy of `CLAUDE.md`.

## Skill → workflow → real commands (verified against the built binary at review)

| Skill | User outcome | Commands |
|---|---|---|
| `inventory-discovery` | "what access do I have?" | `doctor`, `sync <provider>`, `inventory providers/sources/scopes`, `target list` (+ `-o json`) |
| `target-selection` | "point kubectl at the right cluster" | `target list`/`inspect`/`use` (`--no-kubeconfig`)/`current`; `target hide`/`unhide` + `list --all` |
| `label-and-collection-management` | "organize + act on groups" | `target label add/remove/list`, `collection create --selector`/`show`/`use`/`list`/`delete` |
| `credential-inspection` | "which access is valid/expiring?" | `credential list`/`show`/`renew` |
| `sync-and-verify` | "refresh without losing my labels" | `sync <provider>`, verify via `target list`/`current` |
| `release-and-status` | "what am I running / pointed at?" | `version`, `current` |

## Files to change **in this repo** (kuberoutectl)

| File | What changes | Why |
|---|---|---|
| `README.md` | add a pointer to the `kuberoutectl-skills` repo; retire the stale `#43` Roadmap bullet (`README.md:49`) + the Status "AI-skills repo … roadmap" line (`README.md:427`) | discoverability + no stale roadmap contradiction |

## Human / cross-repo prerequisite

- **Create `ymedlop/kuberoutectl-skills`** (public). Either the maintainer creates
  it, or — with confirmation — `gh repo create ymedlop/kuberoutectl-skills --public`
  at build time. This is the one outward action; treat like the tap/bucket setup.

## Tasks

> Most tasks target the NEW repo (worked in a local clone), not this worktree.
> Task 5 is the only change to `kuberoutectl` itself.

### Phase 1 — New repo scaffold

| # | Task | Files (in `kuberoutectl-skills`) |
|---|------|-------|
| 1 | Create/scaffold the repo: `README.md` (what/who/template/conventions/target-version), `LICENSE` (Apache-2.0) | `README.md`, `LICENSE` |

### Phase 2 — The skills (depend on the template)

| # | Task | Files |
|---|------|-------|
| 2 | `inventory-discovery`, `target-selection`, `sync-and-verify` | 3 × `*/SKILL.md` |
| 3 | `label-and-collection-management`, `credential-inspection`, `release-and-status` | 3 × `*/SKILL.md` |

### Phase 3 — Drift guard + cross-link

| # | Task | Files |
|---|------|-------|
| 4 | `verify-commands.yml` — CI that installs a pinned `kuberoutectl` (from the release/apt repo) and asserts each referenced command/flag exists in `--help` | `.github/workflows/verify-commands.yml` (new repo) |
| 5 | kuberoutectl `README.md` — pointer to the skills repo + retire the stale roadmap/status lines | `README.md` (this repo) |

### Parallel vs sequential

| Group | Tasks | Note |
|---|---|---|
| Sequential | 1 → (2, 3) | skills follow the README template |
| Parallel | 2, 3 | independent skill files |
| Parallel | 4, 5 | independent (different repos) |

## Verification / testing plan

| Check | How | Ties to |
|---|---|---|
| Commands real | `verify-commands.yml` (CI) + local `kuberoutectl <cmd> --help` — every referenced command/flag exists | AC "real workflow", "right command"; the drift guard |
| Absolute links resolve | each `https://ymedlop.github.io/kuberoutectl/...` reference is a real published page/anchor (curl / built Pages) | AC "reference docs" (across repos) |
| No duplication | skills link, don't restate `organizing.md`/guides; not a copy of `CLAUDE.md` | AC "reference, don't duplicate" |
| Vendor-neutral | grep skill content for `Claude`/`Anthropic`/`allowed_tools` → none | AC "usable if provider changes" |
| Safety present | each skill flags mutating vs read-only; `delete`/`clear`/`renew` say "confirm with the user" | impl note "operate safely" |
| Concise / progressive | "Use this when" + happy path first | impl note |
| kuberoutectl README | pointer added; stale `#43` roadmap/status lines retired; Jekyll builds | doc coherence |

### Edge cases / accepted caveats

- **Separate-repo drift is real but managed** — absolute doc URLs can rot if the
  Pages site is restructured; the `verify-commands.yml` guard catches *command*
  drift, and each skill notes its verified kuberoutectl version. Doc-URL rot is
  the residual risk (accepted; a link-check could be a later add).
- **The skills repo can't be built in the kuberoutectl worktree** — it's a
  separate clone. The kuberoutectl-side change (Task 5) is a normal PR here.
- **`release-and-status`** = user status (`version`+`current`), not the maintainer
  release process.

## Gate 2 checklist

**Architecture**
- [x] Standalone repo matches the confirmed distribution goal; vendor-neutral markdown
- [x] References (absolute) over duplication; drift guarded by CI, not ignored
- [x] Cleanly separate from the dev `.claude/skills/`; no app/domain code
- [x] One skill per intent

**Task breakdown**
- [x] New-repo files + the one kuberoutectl file listed; each task ≤ 3 files
- [x] Cross-repo prerequisite (create the repo) separated from automatable work
- [x] Dependencies + parallel/sequential marked

**Testing**
- [x] Command-accuracy (CI drift guard), link-resolution, no-duplication, vendor-neutral, safety, conciseness — mapped to ACs
- [x] Edge cases: drift management, cross-repo build, release-and-status scoping

## Accepted caveat (assumption that could be wrong)

A standalone skills repo only pays off if kuberoutectl is actually driven by AI
agents / an ecosystem forms around it (the #44 MCP bet). It's the higher-maintenance
path — chosen deliberately for distributability. The `verify-commands.yml` guard is
what keeps that maintenance cost bounded; without it, a separate repo would silently
rot against the CLI.
