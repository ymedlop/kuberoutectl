# Plan: on-demand update check

**Spec**: `.planning/specs/update-check-on-demand.md`
**Created**: 2026-07-28
**Status**: shipped in #114
**Target release**: 1.2.0

## Stack note

Go CLI. Layering is the repo's own — `domain` ← `providers`/`services` ←
`cli`/`mcpserver`. The new `internal/updatecheck` sits beside `execx` as an
outbound-access adapter: it owns the HTTP call, and `services` reaches it through
an interface it declares, so nothing above `cli` depends on it.

---

## Architecture decisions

### D1 — The config-file opt-out in the spec cannot be built as written

The spec requires the opt-out to be `KUBEROUTECTL_NO_UPDATE_CHECK` **or a config
field**. There is no config file. `internal/config` resolves paths and holds a
`BinaryPaths` map that nothing ever populates from disk — `config.Default()` is
the only construction path, and `internal/cli/root.go:59` is its only caller.

Adding a config-file loader to host one boolean would be a larger feature than
the one being specified, and one nobody asked for. **The environment variable is
the whole opt-out.** If a config file arrives later, this field goes in it for
free; the spec bullet is amended rather than silently half-implemented.

### D2 — Suppression is structural, not conditional

Both suppression conditions — the env var, and a non-stable build — are decided
**before the checker exists**, in the CLI wiring:

```go
doctor := services.NewDoctorService(a.registry, a.resolver, a.requiredBinary)
if updatecheck.Enabled(buildinfo.Version) {
    doctor = doctor.WithUpdateCheck(updatecheck.New(nil))
}
```

This is what makes "no row **and** no HTTP call" one guarantee instead of two
that can drift apart. A conditional inside the checker would leave a code path
where the request happens and the row is dropped — which is the shape of bug
this repo has now fixed twice (#111, and the vacuous assertions in #112).

It also answers the spec's open Gate 2 question: `WithUpdateCheck` is a builder
option, so a future MCP caller reusing `DoctorService` inherits no network call
by construction, and every existing `NewDoctorService` call and test stays
unchanged.

### D3 — `Newer` is stdlib-only and rejects anything it cannot compare

No new module. `MAJOR.MINOR.PATCH`, optional `v` prefix, numeric compare per
field. Anything else — a pre-release suffix, a build suffix, `dev`,
`0.0.0-snapshot-<sha>`, a non-numeric field — returns `ok=false` and produces no
verdict.

Lexical comparison is the bug to avoid: `"1.10.0" < "1.9.0"` as strings. It stays
invisible until the tenth minor release, so the table test carries that case
explicitly.

### D4 — A failed check is a row, an opt-out is no row

Per the spec, and worth restating because it is the easiest thing to collapse
into one branch: an unreachable API produces an `ok` row saying so, while an
opt-out produces nothing. Different because a check that disappears when it fails
is indistinguishable from one that was never wired up — that is the #111 bug —
whereas an opt-out is the user having asked for silence.

Under D2 the opt-out case cannot reach the row-building code at all, so this
distinction is enforced by structure rather than by a conditional.

---

## Components

| Component | Type | Purpose |
|---|---|---|
| `updatecheck.Newer` | pure fn | version comparison, stdlib only |
| `updatecheck.Enabled` | pure fn | env var + build-kind gate, decided before any client exists |
| `updatecheck.Checker` | struct | the `net/http` call, 3s timeout |
| `services.UpdateChecker` | interface | declared by the consumer, so `services` owns no HTTP |
| `DoctorService.WithUpdateCheck` | builder | opt-in attachment |
| `version --check-update` | flag | the deliberate, synchronous path |

## New files

| File | Purpose |
|---|---|
| `internal/updatecheck/updatecheck.go` | `Newer`, `Enabled`, `Checker` |
| `internal/updatecheck/updatecheck_test.go` | table tests + `httptest` server |
| `internal/updatecheck/testdata/release-latest.json` | GitHub payload fixture |
| `internal/cli/updatecheck_test.go` | doctor/version wiring, import-graph guard |

## Files to change

| File | What changes |
|---|---|
| `internal/services/doctor.go` | `UpdateChecker` interface, `WithUpdateCheck`, the check row, revised doc comment |
| `internal/cli/doctor.go` | wire the checker when enabled |
| `internal/cli/version.go` | `--check-update` flag, JSON fields |
| `README.md`, `docs/guides/*` | opt-out + the "only outbound request" statement |
| `CHANGELOG.md` | the entry |
| `scripts/e2e.sh` | doctor row present; MCP handshake still clean |
| `docs/reference/*` | regenerated — a new flag |

---

## Tasks

### Phase 1 — the pure halves (no I/O)

| # | Task | Test |
|---|---|---|
| 1 | `Newer(current, latest) (newer, ok bool)` | table: `1.0.0<1.1.0`; **`1.10.0>1.9.0`**; equal; `v` prefix either side; `dev`; `0.0.0-snapshot-abc`; empty; `1.2`; `1.2.x`; `1.2.0-rc.1` on either side |
| 2 | `Enabled(version string) bool` | env var set to `1`, to `0`, to empty; unset; `dev`; `0.0.0-snapshot-abc`; `1.0.0`. `KUBEROUTECTL_NO_UPDATE_CHECK=0` must **disable** — the spec says any non-empty value, and a user setting `0` expecting "off" must not get "on" |

### Phase 2 — the fetch (depends on 1)

| # | Task | Test |
|---|---|---|
| 3 | `Checker.Latest(ctx)` against an injected base URL | fixture payload decodes to the tag; `prerelease: true` → not ok; 403; 500; malformed body; connection refused; context timeout. Each returns a **display-ready reason**, asserted non-empty |

### Phase 3 — the service (depends on 1, 3)

| # | Task | Test |
|---|---|---|
| 4 | `UpdateChecker` interface + `WithUpdateCheck` + the row | outdated → `warn` naming both versions; equal → `ok`; failed → `ok` naming why; **no checker attached → no row at all**, and the existing provider rows are byte-identical to today's |
| 5 | Row ordering is deterministic and last | provider rows keep their order; the version row is appended, so existing `-o json` consumers see an added element rather than a reordered list |

### Phase 4 — surfaces (depends on 4)

| # | Task | Test |
|---|---|---|
| 6 | `cli/doctor.go` wiring under `Enabled` | with the env var set, a client that **fails the test if called** is never called and no row appears |
| 7 | `version --check-update` + `latest_version` / `update_available` in the JSON view | plain `version` never calls; `--check-update` does; `-o json` shape is additive |
| 8 | Import-graph guard: only `internal/cli` may import `internal/updatecheck` | parses `internal/**` with `go/parser`; fails if `services`, `mcpserver`, `providers` or `domain` import it. This is the only honest way to assert spec outcome 5 — "every other command makes no request" — since those commands hold no injectable client |

### Phase 5 — document and verify (depends on all)

| # | Task | Test |
|---|---|---|
| 9 | `scripts/e2e.sh`: `doctor` shows the row with a stubbed environment; MCP handshake unchanged | shipped binary |
| 10 | README + docs opt-out, the "only outbound request" statement, CHANGELOG, `make docs-reference` **committed** | docs build; CI's stale-reference check |

### Ordering

Sequential: 1 → 3 (the fetch composes the compare) → 4 → 6/7 → 9.
Parallel: 2 with 1; 8 with anything; 10 last.

---

## Testing plan

| Level | Package | Covers |
|---|---|---|
| Pure | `internal/updatecheck` | `Newer` table, `Enabled` table |
| HTTP | `internal/updatecheck` | `httptest` server per failure mode |
| Service | `internal/services` | row per verdict; **absence** when unattached |
| CLI | `internal/cli` | wiring, the never-called client, JSON shape, import guard |
| e2e | `scripts/e2e.sh` | shipped binary; MCP handshake |

**Ladder** — all five rungs. Task 7 adds a flag, so `make docs-reference` is
mandatory and its output committed; that is the rung that failed CI in #110.

**Verify by injection, not inspection.** Before trusting any guard here:
- stub `Newer` to always return `true` and confirm the "equal versions" test fails;
- make `Enabled` always return `true` and confirm the opt-out test fails;
- add an `internal/services` import of `updatecheck` and confirm task 8 fails.

The last one matters most: an import-graph test that passes because it silently
found no files is worth nothing, so it must also assert it parsed a non-zero
number of packages.

**Stated gap for the PR**: no test hits the real GitHub API. The payload shape is
fixture-driven from the documented `releases/latest` response.

---

## Risks

| Risk | Mitigation |
|---|---|
| The import guard passes vacuously (walked nothing) | Asserts a non-zero package count, and is verified by adding a real violating import |
| `Enabled` reads the env var at call time; tests leak state between cases | `t.Setenv`, which restores automatically |
| A 3s timeout makes `doctor` feel hung offline | Accepted and documented; it is the reason no other command checks. `--offline` is deferred to nice-to-have |
| Adding the row changes `doctor -o json` for existing consumers | Appended last, never reordered; task 5 asserts it |
| The first `net/http` in the core sets a precedent | Confined by the import guard, which makes the boundary enforceable rather than a convention |

---

## Gate 2

**Architecture**
- [x] Repo layering respected; `services` declares the interface it consumes, so
      it gains no HTTP dependency.
- [x] Business logic out of Cobra handlers.
- [x] The new outbound boundary is enforced by a test, not by convention.

**Task breakdown**
- [x] Ten tasks, each ≤ 3 files; dependencies stated.
- [x] New files listed.

**Testing**
- [x] Pure, HTTP, service, CLI and e2e levels planned.
- [x] Every spec edge case mapped: 1→2, 2→3, 3→3, 4→3, 5→3, 6→7, 7→6, 8→8, 9→4.
- [x] Three specific injection checks named up front.

**Called out rather than assumed**
- The spec's config-file opt-out is **dropped**, with the reason (D1). The spec
  bullet is amended in the same PR so the two do not disagree.
- `DoctorService`'s "no network calls" doc comment is revised deliberately, in
  the same change that makes it untrue.

**Gate 2: PASSED**
