# Plan: slim the docs site landing page

**Spec**: none (planned from the request "landing page has too much information")
**Epic**: none
**Created**: 2026-07-21
**Status**: shipped in #83
**Stack**: docs only — one Jekyll page (`docs/index.md`). No code, no CSS change.

## Goal

Make the GitHub Pages landing page (`docs/index.md`) read like a **landing page —
orient and route** — not a full manual. After the previous trim it's 174 lines and
still duplicates what the hero, the four cards, the GIF, the README, and the guides
already say. Cut the duplicated prose; keep the strong visual top, one tight "get started"
block, and a single clean "where to next" links block. **Minimalist target: ~70
lines.**

## Diagnosis (current 174-line page)

| Section | Lines | Verdict |
|---------|-------|---------|
| hero + 4 cards + GIF | ~55 | **keep** — the value at a glance |
| `## Why kuberoutectl` (Problem/Solution) | 25 | **shrink to a 1–2 sentence hook** — drop the Problem/Solution sub-headers + bullets; keep a punchy problem framing (hero + cards carry the rest) |
| `## Quick Start` | 12 | **keep**, add an install line so it's the single "how to start" |
| `## Core Concepts` (domain model) | 12 | **→ link** — reference material; lives in the guides + README |
| `## Documentation Structure` | 31 | **compress to a links list** — drop the "Each guide covers" 6-step list and the "Shared Model" paragraph |
| `## Commands` (pointer) | 6 | **fold into the links block** |
| `## Architecture & Design Principles` | 11 | **→ one link** — the 5 bullets live in the README/ARCHITECTURE |
| `## Getting Help` | 7 | **fold into the links block** |
| `## Contributing` | 4 | **→ one link** in the Learn more block |
| `## License` | 2 | **cut** — the hero eyebrow already says "Apache-2.0"; the full text lives in the repo `LICENSE` file (GitHub surfaces it; README links it) |

Nothing is *deleted from the docs* — Core Concepts and the design principles already
live in the README (`#core-concepts`, the boundary bullets) and the guides; the page
stops re-hosting them and links instead.

## Target structure (docs/index.md)

1. Front matter (unchanged)
2. **Hero** div (unchanged) — tagline + CTA buttons + provider strip
3. **Feature grid** — the 4 cards (unchanged)
4. **Demo GIF** (unchanged)
5. **Why** (the hook) — **1–2 sentences only**, no sub-headers, no bullet lists.
   Frames the *problem* (which the hero, being solution-framed, doesn't), landing on
   what kuberoutectl does. Example:
   > Every extra cloud means another CLI, another identity, and a few more unreadable
   > `~/.kube/config` contexts. `kuberoutectl` collapses that into one local inventory
   > of what you can reach and whether it's healthy — so you route `kubectl` to the
   > right cluster in seconds.
6. **Quick Start** — keep this **exact heading** (`## Quick Start` → slug
   `#quick-start`), because the hero's "Quick start" button links to `#quick-start`;
   renaming it would break that button. One tight block: an install line (`brew` +
   "see all methods") **plus** the discover→organize→route loop.
7. **Learn more** — a single tidy links list replacing *Documentation Structure
   + Core Concepts + Commands + Architecture & Design Principles + Getting Help*:
   - Installation (all platforms) → `installation/`
   - Provider guides — Azure · AWS · GCP · kubeconfig → `guides/`
   - Organizing: labels & collections → `organizing/`
   - Full command reference → `README.md#commands`
   - Concepts & architecture → `README.md#core-concepts` / `ARCHITECTURE.md`
   - Contributing → `README.md`

Pin the exact anchors above (don't leave a bare "README") so the builder doesn't
re-derive which section is meant.

Kept: a tiny `## Why` hook (1–2 sentences). Dropped: the Problem/Solution prose,
the Core Concepts list, the Architecture bullets, and the `## License` section — the
hero eyebrow already shows "Apache-2.0", and the full license lives in the repo
`LICENSE` file (surfaced by GitHub, linked from the README one click away).

## Files to change

| File | What changes | Why |
|------|--------------|-----|
| `docs/index.md` | rewrite everything below the GIF to the lean structure above | remove duplication; make it a landing page |

## Files to check (read-only, no change)

| File | Check |
|------|-------|
| `docs/guides/index.md` | confirms the domain model + credential-health spectrum are covered there, so "Core Concepts" can become a link without losing info. If it isn't, keep a one-line concept pointer to the README instead. |
| `README.md` | confirm `#core-concepts` / `#commands` anchors exist (they do post-rewrite) so the links resolve. |

## Tasks

### Phase 1 — verify nothing is lost (read-only)

| # | Task | Files |
|---|------|-------|
| 1 | Confirm the concept model + health spectrum live in `docs/guides/index.md` (or the README), so Core Concepts can be replaced by a link. Note the exact link target. | `docs/guides/index.md`, `README.md` (read) |

### Phase 2 — rewrite (depends on 1)

| # | Task | Files |
|---|------|-------|
| 2 | Rewrite `docs/index.md` below the GIF: a **1–2 sentence `## Why` hook**, then the `## Quick Start` block (install + loop; heading text unchanged so `#quick-start` stays valid for the hero button), then a "Learn more" links block. No `## License`/`## Contributing` sections. | `docs/index.md` |

### Phase 3 — verify

| # | Task | Files |
|---|------|-------|
| 3 | Build + link check (below); fix anything broken. | — |

### Parallel vs sequential

| Group | Tasks | Note |
|-------|-------|------|
| Sequential | 1 → 2 → 3 | verify targets, then rewrite, then check |

## Testing / verification plan

| Check | How | Ties to |
|-------|-----|---------|
| Builds | `cd docs && bundle exec jekyll build` succeeds | it renders |
| Visual top intact | hero, 4 `feature-card`s, and the `demo` GIF still present | keep the value-at-a-glance |
| Anchor not broken | the hero button targets `#quick-start`; that heading must still exist | no dead in-page link |
| Links resolve | every `Learn more` link points at a real page/anchor: `installation/`, `guides/`, `organizing/`, README `#commands` / `#core-concepts`, `ARCHITECTURE.md` | routing works |
| No unique info dropped | Core Concepts + design principles reachable via the README/guides links (Task 1 confirmed the target) | content preserved |
| Materially shorter | `wc -l docs/index.md` ≈ 70 (from 174) | the actual goal |
| No stale internal refs | grep the site for links into the removed sections (`#core-concepts` inside docs, `#getting-help`, `#common-commands`, `#documentation-structure`) → none dangling | no broken nav |

## Gate 2 checklist

**Architecture**
- [x] Single Jekyll page; matches existing docs conventions (hero/cards/links); no CSS/layout change
- [x] Content that's cut is preserved elsewhere (README/guides) and linked, not lost
- [x] Landing-page role (orient + route) vs manual role (guides/README) respected

**Task breakdown**
- [x] Only `docs/index.md` changes; read-only checks listed separately
- [x] Small, one commit; dependencies marked (verify → rewrite → check)

**Testing**
- [x] Build check planned
- [x] Link/anchor resolution planned (incl. the `#quick-start` hero button)
- [x] Content-preservation + no-dangling-refs planned
- [x] The real success metric (length ↓ to ~90) is measurable

## Decision

**Minimalist trim, with a small Why hook** (confirmed with user): keep a 1–2 sentence
`## Why` as a hook, the visual top, one `## Quick Start` block, and a links list —
everything else routes to the README/guides. The reviewer noted that cutting Why
entirely loses "narrative framing"; the tiny hook restores just enough of it without
the bloat.
