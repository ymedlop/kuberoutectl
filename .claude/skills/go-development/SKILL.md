---
name: go-development
description: "Go conventions for kuberoutectl. Use when writing, reviewing, or refactoring Go code — new packages, services, CLI wiring, tests, error handling, or package layout."
allowed_tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
---

# Go Development

Use this skill when working on Go code in `kuberoutectl`.

## Goals

- Keep code small, idiomatic, and testable; clear naming over clever abstractions.
- Keep Cobra handlers thin: parse flags, call services, render. Nothing else.
- Preserve the provider-agnostic architecture (dependency arrows point inward:
  `domain` ← `services` ← `cli`; `domain` imports nothing from this repo).

## Workflow

1. Identify the smallest testable change; write the failing test first (red),
   then the code (green), then refactor.
2. Put logic in a service or provider package, never in a Cobra `RunE`.
3. Verify the ladder before any PR:
   `go test ./...` → `make check` (fmt+vet+test) → `bash scripts/e2e.sh`.

## Gotchas

- **Read projections must copy before filtering.** Filtering a store-backed
  slice in place (`kept := targets[:0]`) mutates the cached snapshot for every
  later reader. `TargetService.all()` copies for exactly this reason — caught
  by a failing selector test. Copy first, then filter/alias.
- **Don't override `HOME` before `go build`.** Scripts that isolate `HOME`
  (like `scripts/e2e.sh`) must build with the real `HOME` first, or the Go
  module cache relocates into the temp dir and cleanup fails with
  read-only permission errors. Isolate `HOME` only for the CLI runs.
- **stdout is the machine contract, stderr is the human channel.** Every
  inventory command supports `-o json` on stdout; progress, warnings, and
  "fetching..." chatter go to stderr (`cmd.ErrOrStderr()`). Mixing them breaks
  piping.
- **If you document a flag, it must exist.** The provider guides once showed
  `credential list --provider` before the flag existed — docs and command
  surface drift apart silently. When adding a command or flag, update
  README.md and assert the behavior in `scripts/e2e.sh` in the same change.
- **Run `gofmt -w` before committing.** `make check` fails on formatting, and
  struct-field alignment is easy to get wrong by hand after edits.
- **Deterministic output everywhere.** Sort slices before returning
  (targets by ID, map keys via `sortedKeys`) — table output and tests both
  depend on stable ordering.

## References

- `references/package-layout.md`
- `references/testing.md`
- `references/errors.md`
- `references/cli-patterns.md`
