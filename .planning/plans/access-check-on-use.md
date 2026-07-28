# Plan: live access check on `target use` and `target inspect`

**Spec**: `.planning/specs/access-check-on-use.md`
**Created**: 2026-07-28
**Status**: draft
**Target release**: 1.2.0
**Builds on**: #112 (the check), #116 (the listing columns)

## Stack note

Go CLI, repo layering: `domain` ← `providers`/`services` ← `cli`/`mcpserver`.
Everything AWS-specific stays inside `internal/providers/aws`; the services layer
reaches it through an optional interface by type assertion, exactly as
`ContextActivator` and `CredentialActivator` are reached today.

---

## Architecture decisions

### D1 — One shared lookup, called by both services

The spec's Gate 2 question. `SelectionService.UseTarget` and
`TargetService` both need the same call with the same inputs, and two entry
points is how their verdicts eventually stop agreeing.

Rejected: a new `AccessService`. It would own no state and have one method —
indirection for its own sake, which CLAUDE.md §2 rules out.

**Chosen**: one unexported function in `internal/services`:

```go
func checkAccess(ctx context.Context, reg *providers.Registry,
    t domain.Target, creds []domain.Credential) providers.AccessCheck
```

It resolves the provider, type-asserts `AccessChecker`, and returns a zero
`AccessCheck` when the provider does not implement it — so "no check attempted"
and "provider cannot check" are the same value and no caller branches on
provider identity. Both services call it; neither reimplements it.

### D2 — `TargetService` takes the registry positionally

The spec corrected a fabricated precedent here. The real one is unambiguous:
`NewSelectionService(store, registry, now)` and `NewCredentialService(store, reg)`
both take it as a constructor argument, and there is no `With…` builder anywhere
in this repo. `NewTargetService(store, reg)` matches.

36 call sites, 2 of them production. Tests that do not exercise access checking
pass `nil`, which `checkAccess` handles as "no provider, no check" — the same
path a kubeconfig target takes.

### D3 — The fold must gain access data without gaining `CredentialIDs`

The widened sync makes single-credential targets carry a verdict. The obvious
edit — delete `foldGroup`'s `if len(group) == 1 { return group[0] }` — also makes
them carry `CredentialIDs`, which silently destroys the property that let #109
ship without a migration and is asserted by
`TestFoldLeavesSingleCredentialTargetsAlone`.

So the early return goes, and the assignment is gated instead:

```go
primary := group[0]
if len(group) > 1 {
    primary.CredentialIDs = ...          // unchanged, still nil for a group of one
}
primary.AccessCheck = access.check       // new: applies to every group size
for _, c := range group {                // iterate the GROUP, not CredentialIDs,
    if access.operable[c.CredentialID] { // which is nil for a group of one
        primary.OperableCredentialIDs = append(...)
    }
}
```

Iterating the group rather than `primary.CredentialIDs` is the part that is easy
to get wrong: the latter is nil in exactly the new case.

### D4 — The selector value is `true`/`false`/`unknown`, not the verdict string

`domain.AccessVerdict` renders as `operable` / `not operable` / `unknown`. The
middle one contains a space, and `-l "operable=not operable"` is unusable. The
selector exposes a three-valued string instead:

| Verdict | `operable` label |
|---|---|
| `AccessOperable` | `true` |
| `AccessNotOperable` | `false` |
| `AccessUnknown` | `unknown` |

Computed from the **primary** credential, because a selector matches a target and
not a (target, credential) pair. Always present — a target nothing was concluded
about is `unknown`, never absent, so `-l operable=unknown` can find it.

### D5 — A second service method rather than a boolean parameter

Every surface defaults to cached and opts in with `--refresh` / `refresh: true`.
Rather than `ResolveWithCredentials(ref, live bool)` — a boolean at a call site
says nothing about what it selects — there are two methods:

- `ResolveWithCredentials(ref)` — unchanged, cached, no network.
- `ResolveWithAccessCheck(ctx, ref)` — the same join plus a live check.

Each caller picks by the flag: `--refresh` and `refresh: true` select the
second, everything else the first. One name, one default, both surfaces.

### D6 — `Resolve` is not touched

Requirement 7. `get_target` currently calls `TargetService.Resolve`, which is
shared with `target label add`/`remove`/`list`. The handler is rewired onto the
join; `Resolve` keeps making no external call. A test asserts the label commands
issue no provider calls, because the wrong implementation here is the natural one.

---

## Components

| Component | Type | Purpose |
|---|---|---|
| `providers.AccessChecker` | optional interface | `CheckAccess(ctx, target, creds) (AccessCheck, error)` |
| `providers.AccessCheck` | struct | `Mode` + `Operable []CredentialID` |
| `aws.Provider.CheckAccess` | provider method | reuses `checkAccessEntries`, `principalKey`, `matchOperable` |
| `services.checkAccess` | unexported fn | the single lookup both services call (D1) |
| `TargetService.ResolveWithAccessCheck` | method | join + live check |
| `Target.SelectionLabels` | domain | gains `operable` (D4) |
| `foldGroup` | aws | access data for every group size (D3) |

## Files to change

| File | What |
|---|---|
| `internal/providers/provider.go` | `AccessChecker`, `AccessCheck` |
| `internal/providers/aws/accessentry.go` | `CheckAccess`, reusing the existing halves; downgrade errors to data |
| `internal/providers/aws/aws.go` | drop the `len(group) > 1` bound; report how many were checked |
| `internal/providers/aws/build.go` | D3 |
| `internal/domain/target.go` | D4 |
| `internal/services/selection.go` | live check in `UseTarget` |
| `internal/services/inventory.go` | registry, `ResolveWithAccessCheck`, `checkAccess` |
| `internal/cli/target.go` | `inspect` live; `use` reports both directions |
| `internal/mcpserver/tools_read.go` | rewire `get_target`, add `refresh` |
| `internal/mcpserver/tools_write.go` | `access_verdict` on `use_target` |
| `scripts/e2e.sh`, `CHANGELOG.md`, `docs/guides/aws.md` | verify + document |

---

## Tasks

### Phase 1 — Domain and interface (no behaviour)

| # | Task | Test |
|---|------|------|
| 1 | `Target.SelectionLabels` gains `operable` (D4) | all three values; **`operable=false` matches only a confirmed refusal** — a naive "not in the set → false" passes the other two and inverts this one; a pre-#112 target is `unknown` |
| 2 | `providers.AccessChecker` + `AccessCheck` | compile-time assertion that `*aws.Provider` implements it |

### Phase 2 — The provider (depends on 2)

| # | Task | Test |
|---|------|------|
| 3 | `aws.Provider.CheckAccess`, reusing `checkAccessEntries`/`matchOperable` | one call for N credentials, asserted on `runner.Calls`; `CONFIG_MAP` makes none; pagination followed |
| 4 | **Never returns an error for a command or parse failure** — downgrades to `unavailable` data, with a reason naming a possible CLI format change for the parse case | edge cases 5, 6; the parse-failure reason must be **distinct** from the routine `unknown` reasons, asserted by comparing the two strings |

### Phase 3 — The widened sync (depends on 3)

| # | Task | Test |
|---|------|------|
| 5 | `foldGroup` sets access data for every group size, `CredentialIDs` only for >1 (D3) | `TestFoldLeavesSingleCredentialTargetsAlone` and the pre-upgrade fixture pass **unmodified** — that is the proof, and editing either means the property broke |
| 6 | Drop the `len(group) > 1` bound in `Discover`; `prog.Step` the count checked | a single-profile cluster now gets a verdict; a `CONFIG_MAP` one still costs no call |

### Phase 4 — Services (depends on 2, 5)

| # | Task | Test |
|---|------|------|
| 7 | `services.checkAccess` (D1); `NewTargetService(store, reg)` (D2) | a nil registry, and a provider without the interface, both yield "not attempted" rather than an error |
| 8 | `ResolveWithAccessCheck` (D5) | live verdict overrides the cached one (edge 12); non-AWS makes no call, asserted on the runner |
| 9 | `UseTarget` performs the check and reports both directions | operable, refused-with-alternative, unknown-is-silent; a failed check does **not** block activation |

### Phase 5 — Surfaces (depends on 7–9)

| # | Task | Test |
|---|------|------|
| 10 | `inspect` live; `use` prints the verdict on stderr | refusal on stderr while the selection line stays on stdout |
| 11 | `get_target` rewired onto the join + `refresh`; label commands untouched (D6) | **the three label commands make no provider call**, asserted on the runner |
| 12 | `use_target` returns `access_verdict` | CLI and MCP agree, from one service call |

### Phase 6 — Verify and document

| # | Task |
|---|------|
| 13 | e2e: `use` and `inspect` against the fake `aws`; no call for the azure target; `-l operable=true` filters |
| 14 | `CHANGELOG.md`, `docs/guides/aws.md`, and the `OPERABLE`-removal note in #116's entry reconciled with the selector |

### Ordering

Sequential: 2 → 3 → 5 → 6; 7 → 8/9 → 10/11/12 → 13.
Parallel: 1 with 2; 4 with 5; 14 with 13.

---

## Testing plan

| Level | Package | Covers |
|---|---|---|
| Domain | `internal/domain` | the three selector values, including the inverted-`false` trap |
| Provider | `internal/providers/aws` | one call for N creds, modes, pagination, both failure downgrades |
| Provider, regression | `internal/providers/aws` | the **unmodified** fold and pre-upgrade tests |
| Services | `internal/services` | nil registry, missing interface, override-not-merge, no-block-on-failure |
| CLI | `internal/cli` | stderr/stdout split, label commands make no call |
| MCP | `internal/mcpserver` | `refresh` default, parity with the CLI |
| e2e | `scripts/e2e.sh` | shipped binary |

**Ladder** — all five rungs. Task 11 adds an MCP argument, not a CLI flag, so
`make docs-reference` should be unchanged — to be confirmed by running it, not
assumed.

**Verify by injection**, before trusting any of these:
- map `AccessNotOperable` to the same selector value as `AccessUnknown` → task 1's
  third case must fail;
- delete `foldGroup`'s `len(group) > 1` guard → the migration tests must fail;
- make `CheckAccess` propagate the parse error → task 9's no-block test must fail;
- point `get_target` back at `Resolve` → task 11 must fail.

**Stated gap**: no test runs against a real EKS cluster. The ARN reduction this
depends on **was** validated manually against the operator's account after #112
(positives matched); the live path here is fixture-driven.

---

## Risks

| Risk | Mitigation |
|---|---|
| The fold change silently breaks the no-migration property | The two existing tests must pass unmodified; injection-verified |
| A parse failure reads like a routine `unknown` | Task 4 asserts the two reason strings differ |
| The label commands acquire a network call | Task 11 asserts on the runner, and D6 keeps `Resolve` untouched |
| Widened sync makes `sync aws` noticeably slower | One call per conclusive-mode cluster, zero for `CONFIG_MAP`; `prog.Step` reports the count so the cost is visible rather than mysterious |
| `nil` registry passed by 34 test call sites hides a real wiring bug | `checkAccess` treats nil as "not attempted", and task 8 asserts the *production* path does attempt it |

---

## Gate 2

**Architecture**
- [x] Provider-specific logic stays in the adapter; services reach it by type
      assertion, no `switch provider`.
- [x] The Gate 1 open question (one lookup or two) resolved as D1, with the
      rejected alternative named.
- [x] D2 derives from the real precedent, after the spec's fabricated one was
      corrected.

**Task breakdown**
- [x] 14 tasks, each ≤ 3 files; dependencies and parallelism stated.
- [x] The two invariants the spec flagged have their own tasks (5, 11) whose
      proof is that existing tests pass untouched.

**Testing**
- [x] Every spec edge case mapped: 1→6, 2→3, 3→9, 4→8, 5/6→4, 7→3, 8→9, 9→9,
      10→8, 11→(accepted), 12→8, 13→11, 14→9, 15→5, 16→1.
- [x] Four injection checks named up front.
- [x] The negative assertions (no call) are on `runner.Calls`, not on output.

**Called out rather than assumed**
- D3 is a change to code merged three PRs ago whose failure mode is silent; it is
  separated from task 6 so the fold change is proven safe before the sync bound
  moves.
- D2 leaves 34 test call sites passing `nil`. That is mechanical, but it means
  the tests cannot catch a production wiring mistake — task 8 covers that gap
  explicitly rather than trusting the constructor change.

**Gate 2: PASSED**
