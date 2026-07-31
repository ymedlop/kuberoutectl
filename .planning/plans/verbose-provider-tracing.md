# Plan: Verbose provider CLI tracing + AWS expired-token visibility

**Spec**: .planning/specs/verbose-provider-tracing.md
**Epic**: none
**Created**: 2026-07-24
**Status**: shipped in #99
**Stack**: Go CLI (Cobra + layered services/providers/execx)

---

## Architecture

Two independent, additive changes that share no new abstraction:

1. **Verbose tracing** lives entirely in `internal/execx` as a `CommandRunner`
   decorator. Providers keep depending on the `CommandRunner` interface and are
   unaware tracing exists — no provider gains a `verbose` field, honoring the
   "keep the core provider-agnostic / no provider conditionals" rule.
2. **AWS auth-failure diagnostic** is a single `prog.Step` in `aws.Discover`,
   reusing data already computed (`stsErr`, `classifyAuth`). No new interface,
   no capability change. It is always-on (progress already streams to stderr).

### Lifecycle constraint (from spec design notes)

`newApp()` builds the runner once, before Cobra parses flags, and injects it
into every provider at registration. So the trace decorator holds a **pointer
to a shared, mutable `TraceConfig`** (`Enabled bool`, `Writer io.Writer`) that
`PersistentPreRunE` flips after parsing — identical lifecycle to how `a.output`
is set today.

### Components

| Component | Type | Purpose |
|-----------|------|---------|
| `TraceConfig` | struct (execx) | Shared mutable toggle: `Enabled` + `Writer`. Held by `app`, flipped in `PersistentPreRunE`. |
| `traceRunner` | CommandRunner decorator (execx) | Wraps inner runner; after each `Run`, writes command + exit status (+ stderr on failure) to `cfg.Writer` when enabled. |
| `authFailureHint` | pure func (aws) | Maps profile + auth type → human diagnostic string (SSO/role → `aws sso login`; static/unknown → verify credentials). |
| `testProgress` | test helper (aws_test) | Capturing `providers.Progress` to assert emitted steps. |

### File Locations (new files)

| File | Location | Purpose |
|------|----------|---------|
| `trace.go` | `internal/execx/` | `TraceConfig` + `traceRunner` + `NewTraceRunner`. |
| `trace_test.go` | `internal/execx/` | Unit tests for the decorator (disabled passthrough, success trace, failure+stderr, empty-stderr). |

### Files to Change

| File | What Changes | Why |
|------|-------------|-----|
| `internal/cli/root.go` | `app` gains `trace *execx.TraceConfig`; `newApp()` builds `traceRunner` wrapping `ExecRunner` and stores the cfg; `rootCmd()` adds persistent `--verbose`/`-v` bool; `PersistentPreRunE` sets `a.trace.Enabled` + `a.trace.Writer = cmd.ErrOrStderr()`. | Wire the flag through the same lifecycle as `--output`. |
| `internal/providers/aws/aws.go` | In `Discover`, when `stsErr != nil`, emit `prog.Step(authFailureHint(profile, authType))` before the `continue`. | The always-on diagnostic (the actual bug fix). |
| `internal/providers/aws/health.go` | Add `authFailureHint(profile, authType string) string`. | Pure mapping, colocated with the other auth/health mapping. |
| `internal/providers/aws/aws_test.go` | Add `testProgress` collector; assert diagnostic emitted for the expired `default` profile and NOT for healthy profiles; assert credential still `HealthExpired`/`ActionRenew`. | Cover the diagnostic without regressing resilience. |
| `scripts/e2e.sh` | Fake `aws` `default` STS case echoes an error to **stderr** before `exit 1`; assert `sync aws` (default output) contains the diagnostic line; add a `sync aws --verbose` run asserting the raw `[exec] … → exit` + stderr trace appears. | End-to-end proof of both behaviors. |
| `docs/reference/*.md` | **Regenerated** via `make docs-reference` — not hand-edited. The persistent `--verbose`/`-v` flag surfaces automatically under "Options inherited from parent commands" on every page + on `kuberoutectl.md`'s own Options. | Keep the generated Cobra reference in sync with the new flag. |
| `README.md` | Document the global `--verbose`/`-v` flag next to the `--output json` note (~line 143); one line on expired-token visibility. English. | README is the entry point; users need to discover the flag. |
| `docs/guides/aws.md` | Add a short troubleshooting note: expired SSO/role tokens now surface a diagnostic on `sync aws`, and `--verbose` shows the raw `aws …` commands + stderr. English. | AWS guide is where the user's actual pain (expired token) is documented. |

No changes to `sync.go`, `syncSummary`, domain types, or persistence.

**Docs are generated where possible.** `docs/reference/` comes from `cmd/gen-docs`
via `internal/cli.RootCommand()`; regenerate with `make docs-reference` rather
than editing those files. Only README and the AWS guide are authored by hand,
and always in **English** (per repo convention).

---

## Trace output format (concrete)

Decorator writes, per invocation, when enabled:

- success: `[exec] <name> <args...> → ok`
- failure with exit code: `[exec] <name> <args...> → exit <n>` then, if stderr
  non-empty, an indented `       stderr: <trimmed stderr>` block.
- failure without an `*exec.ExitError` (e.g. binary not found):
  `[exec] <name> <args...> → error: <err>`.

Exit code extracted via `errors.As(err, &exitErr)` → `exitErr.ExitCode()`.
Empty stderr emits no `stderr:` line (edge case 3). All to `cfg.Writer` (stderr).

---

## Tasks

### Phase 1: execx trace decorator (no CLI/provider deps)

| # | Task | Files |
|---|------|-------|
| 1 | Add `TraceConfig`, `traceRunner`, `NewTraceRunner(inner, *TraceConfig)`; format success/failure/stderr per spec; passthrough of stdout/stderr/err unchanged. | `internal/execx/trace.go` |
| 2 | Unit tests: disabled → no output + identical passthrough; enabled success → `→ ok`; enabled failure (exit code) → `→ exit N` + `stderr:`; failure empty stderr → no `stderr:` block. | `internal/execx/trace_test.go` |

### Phase 2: AWS diagnostic (independent of Phase 1)

| # | Task | Files |
|---|------|-------|
| 3 | Add `authFailureHint(profile, authType)` in `health.go`; call it via `prog.Step` in `Discover` when `stsErr != nil`. | `internal/providers/aws/health.go`, `internal/providers/aws/aws.go` |
| 4 | Add `testProgress` collector; test: expired `default` profile emits a hint step + still yields `HealthExpired`/`ActionRenew`; healthy `prod-sso` emits none; static failure uses non-SSO wording. | `internal/providers/aws/aws_test.go` |

### Phase 3: CLI wiring (depends on Phase 1)

| # | Task | Files |
|---|------|-------|
| 5 | `app.trace` field; `newApp()` wraps runner with `NewTraceRunner`; `--verbose`/`-v` persistent flag; `PersistentPreRunE` sets enabled + writer. | `internal/cli/root.go` |
| 6 | Extend `root_test.go`: `--verbose` accepted on root and `sync`; default absent leaves behavior unchanged (round-trips through `PersistentPreRunE`). | `internal/cli/root_test.go` |

### Phase 4: e2e (depends on Phases 2 & 3)

| # | Task | Files |
|---|------|-------|
| 7 | Fake `aws` default STS → stderr message + `exit 1`; assert diagnostic in `sync aws`; add `sync aws --verbose` asserting `[exec] … exit` + stderr trace. | `scripts/e2e.sh` |

### Phase 5: Documentation (depends on Phase 3; English only)

| # | Task | Files |
|---|------|-------|
| 8 | Regenerate the command reference: `make docs-reference`. Commit the diff (every page gains `-v, --verbose` under inherited options). Do NOT hand-edit generated files. | `docs/reference/*.md` |
| 9 | README: document `--verbose`/`-v` next to the `--output json` note; one line on expired-token visibility. | `README.md` |
| 10 | AWS guide: troubleshooting note on the expired-token diagnostic + `--verbose`. | `docs/guides/aws.md` |

### Parallel vs Sequential

| Parallel Group | Tasks | Why |
|---------------|-------|-----|
| Group A | Phase 1 (1,2) & Phase 2 (3,4) | Different packages, zero shared files. |
| Group B | Phase 5 tasks 9 & 10 | README and AWS guide are independent hand-edits. |

| Sequential | Depends On | Why |
|-----------|-----------|-----|
| Phase 3 (5,6) | Phase 1 | `newApp()` calls `NewTraceRunner`. |
| Phase 4 (7) | Phase 2, Phase 3 | e2e exercises both the diagnostic and `--verbose`. |
| Phase 5 task 8 | Phase 3 | `make docs-reference` must see the `--verbose` flag wired. |

---

## Testing Plan

| Test | Type | Spec tie |
|------|------|----------|
| disabled decorator passthrough identical | infra unit | Edge case 1; happy path |
| enabled success trace line | infra unit | Verbose must-have |
| enabled failure → exit code + stderr | infra unit | Verbose must-have; testing edge |
| failure empty stderr → no stderr block | infra unit | Edge case 3 |
| AWS expired profile → hint step AND HealthExpired/ActionRenew | provider unit | Bug fix; resilience must not regress |
| AWS healthy profiles → no hint | provider unit | Edge case 5 |
| AWS static failure → non-SSO wording | provider unit | Edge case 6 / nice-to-have |
| `--verbose` accepted, default unchanged | cli unit | Must-have flag |
| e2e: diagnostic in `sync aws`; `--verbose` shows raw trace + stderr | e2e | Verification ladder |

Verification ladder (CLAUDE.md): `go test ./...` → `make check` → `bash scripts/e2e.sh`.

---

## Gate 2 checklist

**Architecture**
- [x] Follows existing patterns (decorator on `CommandRunner`; `prog.Step` diagnostic; flag via `PersistentPreRunE` like `--output`).
- [x] Layering respected: execx (infra) ← providers ← cli; no upward calls, no provider conditionals in cli/services.
- [x] New files in correct dirs (`internal/execx`).

**Task Breakdown**
- [x] All changed files listed.
- [x] All new files listed with locations.
- [x] Each task ≤ 3 files, one commit.
- [x] Dependencies explicit (Phase 3←1, Phase 4←2,3).
- [x] Parallel vs sequential marked (Group A parallel).

**Testing**
- [x] Infra (decorator) tests planned.
- [x] Business-logic (AWS diagnostic) tests planned.
- [x] CLI/integration (flag + e2e) tests planned.
- [x] UI tests: N/A (CLI, no UI).
- [x] Spec edge cases 1,3,5,6 covered; 2/4/7 addressed by design (stderr sink; diagnostic gated on `stsErr != nil`; shared toggle).

**Gate 2: PASS**

---

## Notes / caveats (for the PR)

- Fixtures can't prove real `aws` CLI stderr wording; the trace format is
  asserted against the fake, and the real-CLI shape stays an accepted caveat.
- Static-key failures classify as `authUnknown` (classifyAuth needs `stsOK` to
  detect `:user/`), so their diagnostic uses the neutral wording — correct, and
  covered by a test.
