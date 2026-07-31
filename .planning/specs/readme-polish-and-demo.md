# Spec: README polish + demo GIF

**Created**: 2026-07-20
**Status**: shipped in #73
**Author**: Yeray Medina López
**Epic**: none

## Problem

The repo shipped 1.0.0 with a complete but text-heavy README that opens with
three dense prose paragraphs and no visual. A first-time visitor can't tell in
ten seconds what problem `kuberoutectl` solves, that it's stable/installable, or
what using it looks like — the value proposition, install path, and a concrete
usage example are buried below long "why/goals" sections, and there is nothing
visual. There are no badges, so signals a developer scans for (language, license,
release, build health) are missing.

## Goal

A README that lands the value in the first screenful: badges → one-line value
prop → an animated demo of the real flow → a copy-paste quickstart → why it's
different. A visitor understands *what it is, that it's real, and how to start*
without scrolling. The demo is **deterministic and regenerable** (driven by the
committed fake-provider fixtures, not a real cloud), so it can't leak secrets and
can be rebuilt when the CLI evolves.

## User Stories

- **As a developer discovering the repo**, I want to see in one screenful what
  kuberoutectl does, that it's stable, and a real terminal flow, so I can decide
  whether to try it without reading the whole README.
- **As an operator evaluating tools**, I want a copy-paste quickstart (install +
  the discover→organize→route loop), so I can go from zero to routing kubectl in
  a couple of minutes.
- **As a maintainer**, I want the demo generated from committed fixtures via a
  checked-in `.tape`, so regenerating it is a documented, secret-free command
  rather than a manual screen recording.

## Requirements

### Must have

- **Full README rewrite**, re-sequenced top-down:
  1. Title + badges.
  2. One-sentence value proposition.
  3. Animated demo GIF (inline).
  4. Quickstart: install (link to install docs) + the 4-command loop.
  5. "Why kuberoutectl" / what makes it different (the multi-provider,
     inventory + health + labels/collections angle — condensed from today's
     "Why this exists" / "Project goals").
  6. Core concepts (domain model) — kept, condensed.
  7. Providers, distribution, roadmap, status, building-from-source, license —
     kept, condensed and re-ordered.
- **Badges** (honest — no fabricated compatibility claim):
  - Go version — dynamic from `go.mod` (currently 1.25).
  - License — Apache-2.0.
  - Latest release — v1.0.0.
  - CI status — `ci.yml`.
  - Go Report Card.
  - **Providers** badge (AKS · EKS · GKE · kubeconfig) *instead of* a
    "Kubernetes vX compatible" badge — kuberoutectl routes `kubectl` via the
    provider CLIs and pins no Kubernetes API version, so a version-compat badge
    would assert a guarantee we don't test. A "works with your existing kubectl"
    line in prose conveys the true compatibility.
- **Demo GIF** produced by a committed **vhs `.tape`** that stands up the fake
  `az`/`aws`/`gcloud`/`kubectl` on PATH from `internal/providers/*/testdata`
  (the same mechanism as `scripts/e2e.sh`), in a throwaway HOME, and records the
  operator flow:
  ```
  kuberoutectl doctor
  kuberoutectl sync azure
  kuberoutectl sync aws
  kuberoutectl target list
  kuberoutectl target label add <alias> env=prod
  kuberoutectl collection create prod --selector env=prod
  kuberoutectl target use <alias>
  kuberoutectl current
  ```
  Assets committed under `assets/` (or `docs/assets/`): `demo.tape` + `demo.gif`.
- **A `make demo` (or `scripts/demo.sh`) target** that regenerates `demo.gif`
  from `demo.tape`, documented in `RELEASING.md`/`README` so it's reproducible.
- The README must remain **useful if the GIF fails to load** — the static
  quickstart carries the same commands.

### Nice to have

- Alt text on the GIF; a short caption linking to the full guides.
- A one-time size-optimize pass on the GIF (target ≤ ~2.5 MB).
- Theme-friendly terminal colors (legible on GitHub light *and* dark).

### Out of scope

- Recording against real clouds / real accounts (explicitly rejected — secrets
  risk, not reproducible).
- Animated SVG or asciinema-player embeds (don't animate inline in GitHub
  READMEs); GIF only.
- Changing the docs site (`docs/`, GitHub Pages) — this spec is the repo-root
  `README.md` and the demo assets only.
- Any CLI/code behavior change. Docs + assets + one regen script.
- A CI job that rebuilds the GIF (heavy: needs ttyd/ffmpeg). Regeneration stays
  a documented local command; command-accuracy is covered under testing.

## Data Model

N/A — documentation and asset work, no persisted data.

## API Changes

N/A — no CLI surface change.

## UI Changes

The "UI" here is the README's information architecture and the demo asset:

- **New top section order** (see Must-have list). Badges row directly under the
  H1; value prop; demo; quickstart.
- **Demo asset**: `assets/demo.gif` embedded via `![kuberoutectl demo](assets/demo.gif)`
  with alt text; source `assets/demo.tape` committed alongside.
- Existing deeper content is preserved but condensed and re-sequenced; no unique
  information (concepts, provider notes, distribution, roadmap, status, building)
  is dropped.

## Edge Cases

1. **Demo drift** — a GIF is a binary blob the skills' `verify-commands` guard
   can't inspect, so commands can silently go stale. Mitigation: commit the
   `.tape` (readable, greppable) and pin/record the CLI version in the caption;
   the testing criteria verify every command in the `.tape` against the built
   binary.
2. **Secret-looking output** — the fake fixtures include tokens/ARNs/subscription
   IDs. Verify nothing in the recorded frames resembles a *real* secret (the
   fixtures use obviously-fake `aaaaaaaa-…`/`111111111111` values — confirm, and
   scrub if any look plausible).
3. **GIF too large / slow** — an unoptimized recording can be many MB and janky
   on GitHub. Enforce a size budget and keep the run ≤ ~30s.
4. **Badge/endpoint availability** — Go Report Card renders on first request;
   shields/goreportcard are external and can't be previewed from the sandbox
   (network-denied). Verify URLs are well-formed; accept that live rendering is
   confirmed on GitHub, not locally.
5. **Dropped content in a full rewrite** — re-sequencing risks losing a section
   someone linked to (e.g., `#building-from-source` anchors referenced elsewhere,
   including `docs/installation.md`). Inventory existing anchors before rewriting
   and preserve any that are linked.
6. **Light/dark legibility** — a dark-terminal GIF on GitHub's light theme (and
   vice-versa) must stay readable; pick a theme that works on both.

## Testing Criteria

**Happy path**
- README renders on GitHub: all badges resolve, all internal links work, the GIF
  displays inline in the first screenful.
- `make demo` / `scripts/demo.sh` regenerates `demo.gif` from `demo.tape`
  deterministically (documented, runs from committed fixtures, no real cloud).
- Quickstart commands are copy-paste correct and match the real CLI surface.

**Edge cases**
- Every `kuberoutectl …` command in `demo.tape` **and** the README quickstart
  exists in the built binary (reuse the command-existence check approach from the
  skills repo's `verify-commands`; run against `go build` output).
- Grep the recorded output / fixtures for real-secret-shaped strings → none.
- `demo.gif` is within the size budget (≤ ~2.5 MB).
- Every pre-existing README anchor that is linked from elsewhere in the repo/docs
  still resolves after the rewrite (grep for `README.md#…` and in-README
  `](#…)`).
- Badge URLs are well-formed and point at the correct repo/paths (Go 1.25,
  Apache-2.0, release v1.0.0, `ci.yml`).

## Dependencies

- **Recording toolchain** (build-time, not shipped): `vhs`
  (`go install github.com/charmbracelet/vhs@latest`) plus its runtime deps
  `ttyd` and `ffmpeg` (apt). None are currently installed — installing them is
  part of the build. If the toolchain can't be installed in the sandbox, the
  fallback is to author the `.tape` + regen script and record the GIF where the
  toolchain is available, stating that caveat in the PR (rather than shipping a
  fabricated asset).
- **Fake-provider fixtures** — `internal/providers/{azure,aws,gcp,kubeconfig}/testdata`
  (exist; already drive `scripts/e2e.sh`).
- **Existing README** — the content source to preserve during the rewrite.
- **Accurate facts** — module `github.com/ymedlop/kuberoutectl`, Go 1.25,
  Apache-2.0, latest release v1.0.0, CI workflow `ci.yml`.

## Gate 1 checklist

- [x] Problem clearly stated (specific: value/install/example buried, no badges,
      no visual)
- [x] Goal specific (value in first screenful; deterministic regenerable demo)
- [x] At least one user story (three)
- [x] Requirements split into must / nice / out of scope
- [x] Out-of-scope section exists (real-cloud recording, SVG/asciinema, docs
      site, code changes, GIF-in-CI)
- [x] Edge cases listed (six)
- [x] Testing criteria for happy path
- [x] Testing criteria for edge cases
- [x] Dependencies listed
- [x] Data model / API sections addressed (N/A, justified)
