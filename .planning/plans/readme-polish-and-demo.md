# Plan: README polish + demo GIF

**Spec**: .planning/specs/readme-polish-and-demo.md
**Epic**: none
**Created**: 2026-07-20
**Status**: shipped in #73
**Stack**: Go CLI repo — but this task is **docs + tooling** (Markdown, shell, a
vhs tape). No Go code changes. Phase structure is asset → docs → verify, not
DB→API→UI.

## Confirmed decisions

- **Full README rewrite** (re-sequenced top-down).
- **Deterministic GIF** from a committed vhs `.tape` driving the fake
  `az`/`aws`/`gcloud`/`kubectl` fixtures (same mechanism as `scripts/e2e.sh`).
- **Honest badges** — Go 1.25, Apache-2.0, release v1.0.0, CI, Go Report Card,
  and a **providers** badge (AKS · EKS · GKE · kubeconfig) instead of a fabricated
  "Kubernetes vX" compatibility claim.
- **Asset location: `assets/`** at repo root (the GIF serves the repo-root README,
  not the Pages site). `demo.tape` + `demo.gif` live together there.

## Architecture / artifacts

| Artifact | Type | Purpose |
|----------|------|---------|
| `assets/demo.tape` | vhs script | the demo's source of truth — typed commands, timing, theme |
| `scripts/demo.sh` | shell | stands up the fake provider CLIs + isolated HOME (like `e2e.sh`), then runs `vhs assets/demo.tape` to emit `assets/demo.gif` |
| `assets/demo.gif` | generated binary asset | the embedded demo |
| `make demo` | Makefile target | one-command regeneration → `scripts/demo.sh` |
| `README.md` | full rewrite | new information architecture + badges + demo + quickstart |

### File locations

| File | Location | Purpose |
|------|----------|---------|
| `demo.tape` | `assets/` | new — vhs recording script |
| `demo.gif` | `assets/` | new — generated demo |
| `demo.sh` | `scripts/` | new — deterministic regen wrapper |

### Files to change

| File | What changes | Why |
|------|--------------|-----|
| `README.md` | full rewrite (see IA below) | spec goal: value in the first screenful |
| `Makefile` | add `demo:` target; add `demo` to `.PHONY` | one-command, documented regeneration |
| README `## Development workflow` | one short "Regenerating the demo" note | spec: regeneration must be documented. *(Not RELEASING.md — that file is release-pipeline only; the existing Development workflow section is the right home for local contributor tooling.)* |

### Target README information architecture (top-down)

1. `# kuberoutectl` + **badge row**: Go version (**dynamic** — the shields.io
   `github/go-mod/go-version/ymedlop/kuberoutectl` endpoint, which auto-tracks
   `go.mod` so it never goes stale on a bump — NOT a hardcoded "Go 1.25" string),
   Apache-2.0 (`github/license/…`), release (`github/v/release/…`, currently
   v1.0.0), CI (`actions/workflows/ci.yml/badge.svg`), Go Report Card, providers
   (static: AKS·EKS·GKE·kubeconfig).
2. One-sentence value proposition.
3. **Demo GIF** (`![kuberoutectl demo](assets/demo.gif)` + alt text/caption).
4. **Quickstart** — install (link to install docs) + the discover→organize→route
   loop as copy-paste commands.
5. **Why kuberoutectl** — the multi-provider inventory + health + labels/collections
   differentiator (condensed from today's "Why this exists" / "Project goals"),
   **including the two boundary statements from today's "Design principles"** that
   appear nowhere else: *no secret vault by default* and *managed CLI runtime is
   optional, not the default* (these mirror `AGENTS.md`; they must not be dropped).
6. **Core concepts** — domain model (condensed; keep).
7. **Labels & collections** example (condensed; keep).
8. **Installation** — condense to a pointer to `docs/installation/` + the shortest
   per-platform lines (the full matrix already lives in the docs site).
9. **Providers / guides**, **Distribution**, **Roadmap**, **Status**,
   **Building from source**, **Development workflow**, **Acknowledgements**,
   **License** — kept, condensed, re-ordered.

**Anchors that MUST survive** (verified as linked): `#building-from-source`
(from `docs/installation.md` and in-README line 236) and `#roadmap` (in-README
line 431). Keep those two section headings' slugs unchanged.

**Content-preservation note:** the "Design principles" section
(current `README.md:67-73`) is *not* redundant — fold its two unique boundary
bullets into item 5 above (see the explicit callout there). "What 1.0.0 includes"
and "Distribution" *are* near-duplicates of "Status"/"Installation" and can be
folded losslessly.

## Tasks

> Sequential across phases: the README (Phase 2) embeds the GIF built in Phase 1;
> verification (Phase 3) needs both.

### Phase 1 — Demo asset (tooling first)

| # | Task | Files |
|---|------|-------|
| 1 | `scripts/demo.sh` — port `e2e.sh`'s fake-CLI + isolated-HOME setup into a wrapper that runs `vhs "$ROOT/assets/demo.tape"`. **Safety-critical (see below):** `export HOME=<throwaway>` and **prepend** the fake-bin dir to `PATH` in the *same script process* that then execs `vhs` last — vhs's recorded PTY shell inherits the process env, so the fakes only take effect if exported before `vhs` runs. **Add a fail-fast pre-flight guard**: assert `command -v az` (and aws/gcloud/kubectl) resolves *inside the throwaway bin dir*, not a system path — abort the recording if not, so a mis-ordered `PATH` can never silently tape real-account output. Install toolchain if missing (`go install github.com/charmbracelet/vhs@latest`; `ttyd`+`ffmpeg` via apt); fail with a clear message if it can't. | `scripts/demo.sh` |
| 2 | `assets/demo.tape` — vhs settings (self-contained dark theme legible on light+dark, sane width/height/font, `TypingSpeed`), then the flow: `doctor`, `sync azure`, `sync aws`, `sync gcp`, `sync kubeconfig`, `target list`, `credential list` (health spectrum), `target label add <name> env=prod`, `collection create prod --selector env=prod`, `target use <name>`, `current`. **This extends the spec's literal 8-command list with `sync gcp`/`sync kubeconfig`/`credential list` on purpose** — 4-provider breadth + the credential-health spectrum are kuberoutectl's actual differentiators, and showing only 2 providers undersells them. To stay near the spec's ≤~30s budget with 11 commands, use a fast `TypingSpeed` (~50–75ms) and short `Sleep` beats (sync/list outputs are one-shot and quick); if it lands ~30–40s that's an accepted trade for showing the full value. Task 3 confirms the actual duration/size. | `assets/demo.tape` |
| 3 | Generate `assets/demo.gif` via `make demo`; add the `demo:` Makefile target (+ `.PHONY`). Confirm size ≤ ~2.5 MB (tune tape/scale if over). If the toolchain cannot be installed in this environment, ship Tasks 1–2 + the target and record the GIF where vhs runs — state the caveat in the PR rather than committing a hand-faked asset. | `assets/demo.gif`, `Makefile` |

### Phase 2 — README rewrite (depends on the GIF existing)

| # | Task | Files |
|---|------|-------|
| 4 | Rewrite `README.md` to the IA above: badge row, value prop, demo embed, quickstart, then condensed/re-sequenced body. Preserve `#building-from-source` and `#roadmap` slugs and every piece of unique info. | `README.md` |
| 5 | Add the "Regenerating the demo" note (points at `make demo`, notes it's fixture-driven and secret-free). | README `## Development workflow` section |

### Phase 3 — Verify

| # | Task | Files |
|---|------|-------|
| 6 | Run the verification suite (below) and fix any failures. No shipped file unless a check is worth keeping. | — |

### Parallel vs sequential

| Group | Tasks | Note |
|-------|-------|------|
| Parallel | 1, 2 | `demo.sh` and `demo.tape` are independent files |
| Sequential | 3 → 1,2 | generation needs both the wrapper and the tape |
| Sequential | 4 → 3 | README embeds the generated GIF |
| Sequential | 5 → 4 | doc note rides with the rewrite |
| Sequential | 6 → all | verification is last |

## Testing / verification plan

Mapped from the spec's testing criteria (docs/tooling equivalents of the
data/logic/UI layers):

| Check | How | Spec tie |
|-------|-----|----------|
| **Deterministic regen** | `make demo` twice produces a valid GIF from committed fixtures, no real cloud, no network to a provider | happy path; "deterministic + regenerable" |
| **Command accuracy** | every `kuberoutectl …` in `demo.tape` **and** the README quickstart exists in the freshly `go build`'d binary (reuse the fenced-line + `--help` existence approach from the skills repo's `verify-commands`) | edge case 1 (demo drift); happy path |
| **No real-account output** | two layers: (a) the Task-1 pre-flight guard proves the recording ran against the fake bins, not real CLIs; (b) grep the recorded frames for token/key/real-ID *shapes* AND assert that **only the known fixture identifiers appear** (e.g. `aaaaaaaa-…`, `111111111111`, `AKIAEXAMPLE`) — a positive allowlist, since a contributor's real subscription/cluster *name* isn't secret-shaped and a shape-only grep would miss it | edge case 2; spec out-of-scope "no real clouds" |
| **Size budget** | `demo.gif` ≤ ~2.5 MB | edge case 3 |
| **Badge URLs well-formed** | each badge URL points at the correct repo/path; the **Go badge is the dynamic `github/go-mod/go-version/…` endpoint** (auto-tracks `go.mod`, currently renders 1.25 — *not* a hardcoded string); Apache-2.0, v1.0.0, `ci.yml` correct. Live rendering confirmed on GitHub, not sandbox (network-denied) | edge case 4; spec "Go version — dynamic from go.mod" |
| **Anchor preservation** | after rewrite, `#building-from-source` and `#roadmap` headings still exist; `grep` the repo/docs for those refs and confirm each resolves | edge case 5 |
| **Content preservation** | diff the old section list against the new — every unique fact still present, **explicitly including the two "Design principles" boundary bullets** (no secret vault by default; optional managed runtime), plus concepts, providers, distribution, roadmap, status, building, dev workflow, license, acknowledgements | full-rewrite risk; spec "no unique info dropped" |
| **Light/dark legibility** | tape theme is a self-contained dark terminal that reads on both GitHub themes | edge case 6 |
| **Markdown renders** | README preview: badges resolve, internal links work, GIF embeds inline in the first screenful | happy path |

## Gate 2 checklist

**Architecture**
- [x] Matches repo conventions: assets under `assets/`, scripts under `scripts/`,
      `make` target like existing ones; demo reuses the proven `e2e.sh` fake setup
- [x] Asset built before the doc that embeds it; verification last
- [x] New files placed in the right directories

**Task breakdown**
- [x] All changed files listed (`README.md`, `Makefile`, doc note)
- [x] All new files listed with locations (`assets/demo.tape`, `assets/demo.gif`,
      `scripts/demo.sh`)
- [x] Each task ≤ 3 files, one commit
- [x] Dependencies + parallel/sequential marked
- [x] Toolchain-unavailable contingency captured (Task 3)

**Testing**
- [x] Regeneration (data-layer equivalent) planned
- [x] Command-accuracy (logic equivalent) planned
- [x] Render/format (UI equivalent) planned
- [x] All six spec edge cases covered (drift, secrets, size, badges, anchors,
      legibility)

## Accepted caveat

The demo GIF is a binary the skills' automated drift guard can't inspect; the
committed `.tape` + the Phase-3 command-accuracy check are the mitigation, but
keeping the GIF visually in sync after a big CLI change is still a manual
`make demo`. Acceptable for a README asset — regeneration is one command.
