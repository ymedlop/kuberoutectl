# Plan: AWS access-entry operability

**Spec**: `.planning/specs/aws-access-entry-operability.md`
**Epic**: none
**Created**: 2026-07-27
**Status**: draft
**Target release**: 1.2.0
**Builds on**: #109 (the fold) and #110 (`--profile` selection), both merged

## Stack note

Go CLI — no `build.gradle.kts`, no `package.json`. The skill's
Controller → Manager → Repository template does not apply and is deliberately not
used. Layering is the repo's own:

```
internal/domain     pure types
      ↑
internal/providers  adapters behind Registry + Capabilities; external CLIs via execx
      ↑
internal/services   provider-agnostic business logic
      ↑
internal/cli        Cobra wiring        internal/mcpserver   MCP wiring
```

Every AWS-specific decision stays inside `internal/providers/aws`. `internal/services`
gains nothing in this feature.

---

## Architecture decisions

### D1 — The check must run *before* primary selection, so the fold splits in two

This is the structural constraint the spec could not see, and it is the main
piece of work.

The spec requires operability to outrank health when picking the primary. But
`foldTargetsByID` (`build.go:114`) today **selects the primary and discards the
other candidates' structs**, keeping only their `CredentialID`s — because D4 (from
the previous plan) requires the folded target to *be* the winner's own struct
rather than one assembled from parts.

So "fold first, then re-rank once operability is known" is impossible: by then
the losing candidates' `SystemLabels`, `Metadata`, `Health` and `ActionHint` are
gone, and re-picking would mean assembling a target from parts — precisely the
bug class D4 exists to prevent.

The check also cannot move earlier than grouping: it only runs for clusters
reached by more than one profile, which is what grouping establishes.

**Therefore the fold splits into three steps**, with I/O in the middle:

```
groupTargetsByID(targets)        pure    → ordered groups
  ↓
checkAccessEntries(groups, …)    I/O     → per-group verdict (aws.go)
  ↓
foldGroup(group, verdict)        pure    → the folded target
```

`foldTargetsByID` is retained as a thin wrapper over group+fold with an empty
verdict, so the no-check path (and its existing tests) stay intact.

### D2 — The authentication mode rides on the target as provider metadata

`accessConfig.authenticationMode` comes from `describe-cluster`, which runs
per profile *inside* `discoverClusters` — long before the fold knows whether a
cluster has more than one profile. It cannot be written to `Target.AccessCheck`
there: an empty `AccessCheck` means "not attempted", and a single-profile target
must keep it empty even though its mode is known.

So `buildTarget` stores the raw mode in `Metadata["authentication_mode"]`
(provider-private, like `profile`/`account`/`status` already there), and the
post-group step converts it into `AccessCheck` only for groups it actually
checks. Safe under D4: the mode is a cluster property, identical across every
candidate in a group, so it survives the winner-copy unchanged.

### D3 — Principal matching is asymmetric, and that is the whole trap

An access entry names an IAM principal; STS returns an assumed-role. Reduce both
to **account + final role/user name**:

| Source | ARN | Key |
|---|---|---|
| STS, SSO profile | `arn:aws:sts::123:assumed-role/AWSReservedSSO_Plat_ab/yeray` | `123/AWSReservedSSO_Plat_ab` |
| STS, IAM user | `arn:aws:iam::123:user/ci-bot` | `123/ci-bot` |
| Entry, SSO role | `arn:aws:iam::123:role/aws-reserved/sso.amazonaws.com/eu-central-1/AWSReservedSSO_Plat_ab` | `123/AWSReservedSSO_Plat_ab` |
| Entry, plain role | `arn:aws:iam::123:role/PlatformAdmin` | `123/PlatformAdmin` |

**The extraction direction differs by resource type:**

- `assumed-role/NAME/session` → take the **first** segment after the type.
- `role/…path…/NAME` and `user/…path…/NAME` → take the **last** segment.

Getting this symmetric — stripping to the last segment in both — silently breaks
the SSO case, which is the only case this feature exists for. It is a pure
function with no I/O and gets its own table test before anything calls it.

### D4 — Only a negative answer depends on the mode

Carried from the spec, restated because it is the easiest thing to implement
backwards:

| Mode | Listed | Absent |
|---|---|---|
| `API` | operable | **not operable** |
| `API_AND_CONFIG_MAP` | operable | **unknown** |
| `CONFIG_MAP` | *(no entries)* | **unknown** |

A single `listed/not-listed` boolean is therefore not enough — the verdict is
derived from membership **plus** `AccessCheck`, which is why the domain stores
those two and not a per-pair verdict (spec, Data model).

---

## Components

| Component | Type | Purpose |
|-----------|------|---------|
| `Target.OperableCredentialIDs` | domain field | credentials confirmed to hold an access entry |
| `Target.AccessCheck` | domain field | what the check could establish, or empty if not attempted |
| `awsCluster.AuthenticationMode` | parse struct field | from `accessConfig.authenticationMode` |
| `parseAccessEntries` | pure fn (aws) | `list-access-entries` JSON → ARNs + nextToken |
| `principalKey` | pure fn (aws) | ARN → `account/name`, asymmetric by resource type (D3) |
| `groupTargetsByID` | pure fn (aws) | ordered groups, split out of the fold (D1) |
| `foldGroup` | pure fn (aws) | rank by operability-then-health, return the winner's struct |
| `accessVerdict` | pure fn (aws or services) | (membership, AccessCheck) → operable / not / unknown |
| `Provider.checkAccessEntries` | provider method | the paginated call, per multi-profile cluster |

## New files

| File | Purpose |
|------|---------|
| `internal/providers/aws/accessentry.go` | `principalKey`, `parseAccessEntries`, `checkAccessEntries` |
| `internal/providers/aws/accessentry_test.go` | table tests for the pure halves |
| `internal/providers/aws/testdata/access-entries-*.json` | one fixture per mode + a paginated pair |

## Files to change

| File | What changes | Why |
|------|-------------|-----|
| `internal/domain/target.go` | `+ OperableCredentialIDs`, `+ AccessCheck` | spec Data model |
| `internal/providers/aws/parse.go` | `awsCluster` gains `AuthenticationMode` from `accessConfig` | D2 |
| `internal/providers/aws/build.go` | fold split into `groupTargetsByID` + `foldGroup`; ranking gains operability | D1 |
| `internal/providers/aws/aws.go` | call the check between grouping and folding; progress steps | D1 |
| `internal/cli/target.go` | `OPERABLE` column; per-profile verdict in `inspect`; stderr warning on `use` | spec Surfaces |
| `internal/mcpserver/tools_write.go` | `access_warning` on `use_target` output | spec API changes |
| `scripts/e2e.sh` | access-entry fixtures per mode against the shipped binary | spec Testing |
| `CHANGELOG.md`, `docs/guides/aws.md` | document the check and the `unknown` cases | release hygiene |

---

## Tasks

### Phase 1 — Domain and parsing (no behaviour)

| # | Task | Files | Test |
|---|------|-------|------|
| 1 | `Target.OperableCredentialIDs` and `AccessCheck`, both `omitempty` | `internal/domain/target.go` | JSON round-trip with and without both keys (edge case 13) |
| 2 | `awsCluster.AuthenticationMode` from `accessConfig`; `buildTarget` stores it in `Metadata["authentication_mode"]` | `internal/providers/aws/parse.go`, `build.go` | fixture per mode decodes; a `describe-cluster` payload with **no** `accessConfig` yields "" and does not error |

### Phase 2 — The pure halves (depends on 1)

| # | Task | Files | Test |
|---|------|-------|------|
| 3 | `principalKey` — D3's asymmetric extraction | `internal/providers/aws/accessentry.go` | table test: edge cases 4, 5, 6, plus a malformed ARN, an empty string, and an ARN with too few colons. **Must include the SSO path case and the plain-role case in one table**, since a symmetric implementation passes one and fails the other |
| 4 | `parseAccessEntries` — `{accessEntries:[…], nextToken}` | `internal/providers/aws/accessentry.go` | fixture decode; a malformed body is a **wrapped hard error**, per the provider error convention (edge case 10) |
| 5 | `accessVerdict(listed bool, check string)` — D4's truth table | `internal/providers/aws/accessentry.go` | one case per cell of the 3×2 table (edge cases 2, 3) |

### Phase 3 — Fold restructure (depends on 2)

| # | Task | Files | Test |
|---|------|-------|------|
| 6 | Split `foldTargetsByID` into `groupTargetsByID` + `foldGroup`; keep the old name as a wrapper so existing tests pass unchanged | `internal/providers/aws/build.go` | the whole existing `fold_test.go` must pass **untouched** — that is the proof the refactor is behaviour-preserving |
| 7 | `foldGroup` ranks operability-then-health, and sets `OperableCredentialIDs` + `AccessCheck` | `internal/providers/aws/build.go` | edge case 11: an expired-but-operable profile beats a valid-but-not-operable one. Plus D4's rule that the returned target is still the winner's own struct — assert `SystemLabels` as a whole map against the winner, cloned before the call |

### Phase 4 — The check (depends on 2, 3)

| # | Task | Files | Test |
|---|------|-------|------|
| 8 | `checkAccessEntries` — one `list-access-entries` per multi-profile cluster, `nextToken` followed to exhaustion | `internal/providers/aws/accessentry.go` | edge case 8: a two-page response, with the sought principal on page 2 |
| 9 | Wire it into `Discover` between grouping and folding; `prog.Step` per cluster checked and per mode skipped | `internal/providers/aws/aws.go` | edge case 1: a single-profile cluster triggers **zero** `list-access-entries` calls — assert on `runner.Calls`, not on the result |
| 10 | Resilience: a failed call sets `AccessCheck = "unavailable"` and continues, naming the likely missing permission | `internal/providers/aws/accessentry.go`, `aws.go` | edge case 9; and edge case 14, an empty STS identity yielding `unknown`, never a negative |

### Phase 5 — Surfaces (depends on 3, 4)

| # | Task | Files | Test |
|---|------|-------|------|
| 11 | `OPERABLE` column on `target list`, under the same "only when some target has a choice" rule as `PROFILES` (`target.go:87-101`) | `internal/cli/target.go` | column absent for a single-profile fleet; `unknown` rendered, never blank |
| 12 | `target inspect`: `Access check` line + per-profile verdict | `internal/cli/target.go` | golden output for all three verdicts |
| 13 | `target use`: stderr warning when the chosen profile is **confirmed absent**, naming one that was operable. Silent on `unknown` | `internal/cli/target.go` | warning present for a confirmed negative, **absent** for `unknown` — the second assertion is the one that matters |
| 14 | MCP `use_target` returns `access_warning` | `internal/mcpserver/tools_write.go` | CLI and MCP agree on when a warning is warranted |

### Phase 6 — Verify and document (depends on all)

| # | Task | Files | Test |
|---|------|-------|------|
| 15 | e2e: fake `aws` gains `list-access-entries` for the shared cluster (mode `API`, `ops` listed, `prod-sso` not) and a `CONFIG_MAP` cluster | `scripts/e2e.sh` | the full flow against the shipped binary, including that the `CONFIG_MAP` cluster reads `unknown` |
| 16 | `CHANGELOG.md` + `docs/guides/aws.md`: the check, the three modes, and why `unknown` is common | `CHANGELOG.md`, `docs/guides/aws.md` | docs build |

### Parallel vs sequential

| Parallel group | Tasks | Why |
|---------------|-------|-----|
| A | 3, 4, 5 | three independent pure functions in one new file |
| B | 11, 12 | different render blocks; 13 touches the same file, so it follows |
| C | 15, 16 | independent |

| Sequential | Depends on | Why |
|-----------|-----------|-----|
| 6 | 2 | the mode must be on the target before the fold can read it |
| 7 | 5, 6 | ranking needs the verdict function and the split |
| 8 | 3, 4 | the check composes both pure halves |
| 9 | 7, 8 | wiring needs both sides to exist |
| 11–14 | 7, 9 | render what discovery recorded |
| 15 | 9, 13 | e2e drives the shipped binary |

---

## Testing plan

| Level | Package | Covers |
|-------|---------|--------|
| Domain | `internal/domain` | round-trip with/without the two fields (13) |
| Provider, pure | `internal/providers/aws` | `principalKey` (4, 5, 6, malformed), `parseAccessEntries` (10), `accessVerdict` (2, 3), `foldGroup` ranking (11) |
| Provider, FakeRunner | `internal/providers/aws` | no call for single-profile clusters (1), pagination (8), denied permission (9), empty identity (14), all-absent (12) |
| Provider, regression | `internal/providers/aws` | the **existing** `fold_test.go`, unmodified, proving task 6 preserved behaviour |
| CLI | `internal/cli` | column conditionality, three verdicts in `inspect`, warning present/absent |
| MCP | `internal/mcpserver` | `access_warning` parity with the CLI |
| e2e | `scripts/e2e.sh` | shipped binary, `API` and `CONFIG_MAP` clusters side by side |

**Ladder** — `go test ./... -count=1` → `make check` → `bash scripts/e2e.sh`, **plus
`make docs-reference`** if a flag is added. The local ladder does not cover the
generated command reference; a flag change passes all three rungs and fails CI.

**Stated gap for the PR**: no test runs against a real EKS cluster. The mode
table, the ARN matching and the pagination are fixture-driven from documented
response shapes. **The central assumption — the exact ARN form of SSO access
entries in the target account — must be validated manually against one real
cluster before merging.** If it is wrong, `principalKey` is wrong and the feature
reports confident nonsense.

---

## Risks

| Risk | Mitigation |
|------|-----------|
| The fold split (task 6) changes behaviour subtly | The existing `fold_test.go` must pass **unmodified**. If a test needs editing, the refactor is not behaviour-preserving and the reason must be understood, not accommodated |
| `principalKey` implemented symmetrically passes the plain-role case and fails SSO | Both shapes live in one table test (task 3), so a symmetric implementation cannot go green |
| A confident false negative | Only `API` mode yields a negative; `accessVerdict` is a standalone function with a test per cell so the asymmetry cannot be shortcut |
| Same role name under different IAM paths matches falsely | Accepted and documented (spec, case 7). If that shape exists in the target fleet, matching must become path-aware — a spec revision, not a patch |
| Extra API calls slow down `sync` | Bounded to multi-profile clusters, one call per cluster (not per profile). Task 9 asserts zero calls for the single-profile case |
| Pre-existing: `discoverClusters` treats a parse failure on a **successful** `describe-cluster` as resilient, inverting the repo's error convention | **Not fixed here** — flagged in #109's review as pre-existing. Task 4 must not copy that shape: `parseAccessEntries` failing on a successful call is a hard error |

---

## Gate 2

**Architecture**
- [x] Follows the repo's layering; the Controller/Manager/Repository template is explicitly rejected as inapplicable and the real one substituted.
- [x] Each layer calls only downward — `internal/services` is untouched; every AWS concept stays in the adapter.
- [x] Components in the right directories; one new file in the provider package, no new package.

**Task breakdown**
- [x] All files to change listed — 8 changed, 3 new.
- [x] New files listed with locations.
- [x] Each task ≤ 3 files.
- [x] Dependencies explicit, with the reason on each edge.
- [x] Parallel vs sequential marked — three parallel groups, six ordering constraints.

**Testing**
- [x] Data-layer tests planned (domain round-trip, pre-upgrade decode).
- [x] Business-logic tests planned (three pure functions, each with its own table).
- [x] Integration tests planned (FakeRunner over `Discover`, CLI golden output, MCP parity, e2e).
- [x] UI tests — not applicable; the CLI equivalent is golden output, planned.
- [x] Every spec edge case mapped: 1→9, 2/3→5, 4/5/6→3, 7→accepted risk, 8→8, 9→10, 10→4, 11→7, 12→FakeRunner, 13→1, 14→10.

**Called out rather than assumed**
- D1 is a real refactor of code merged two PRs ago, not an addition. It is the
  largest single risk here and the reason task 6 is separated from task 7: the
  split must be proven behaviour-preserving *before* new ranking is layered on.
- The spec's open risk (case 7, duplicate role names across paths) is carried
  forward unresolved, by decision, and named in Risks.
