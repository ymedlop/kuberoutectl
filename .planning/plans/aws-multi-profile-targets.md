# Plan: AWS multi-profile targets

**Spec**: `.planning/specs/aws-multi-profile-targets.md`
**Epic**: none
**Created**: 2026-07-27
**Status**: shipped in #109 + #110
**Target release**: 1.2.0

## Stack note

Stack detection finds no `build.gradle.kts` and no `package.json` — this is a Go
CLI. The skill's **Controller → Manager → Repository** template does not apply and
is deliberately not used. The layering this plan follows is the one `AGENTS.md`
and `ARCHITECTURE.md` already mandate:

```
internal/domain     pure types, no dependencies
      ↑
internal/providers  adapters behind Registry + Capabilities; external CLIs via execx
      ↑
internal/services   business logic, persistence, provider-agnostic
      ↑
internal/cli        Cobra wiring only          internal/mcpserver  MCP wiring only
```

Rule enforced throughout: **no provider conditional enters `internal/services`.**
Every AWS-specific decision lands in `internal/providers/aws` or behind an
optional provider interface.

---

## Architecture decisions

Two choices the spec left open. Both are recorded here so the builder does not
re-decide them silently.

### D1 — How activation reaches a non-primary credential

`SelectionService.activate` calls `activator.Activate(ctx, target)`, and the AWS
adapter reads `target.Metadata["profile"]` (`activate.go:24`). To activate through
a *chosen* credential, three options were considered:

| Option | Cost | Verdict |
|--------|------|---------|
| Service rewrites `target.Metadata["profile"]` on a copy before calling `Activate` | 0 provider changes, but `services` would encode the knowledge that AWS steers off a `profile` metadata key — a provider-specific assumption in the provider-agnostic layer | **rejected**, violates the layering rule |
| Change `ContextActivator.Activate` to take a `domain.Credential` | one concept, but 4 provider signature changes for a case only AWS has | rejected |
| **New optional interface `CredentialActivator`** | 1 new interface, implemented only by AWS; azure/gcp/kubeconfig untouched | **chosen** |

`ContextActivator` already establishes this exact idiom, and `provider.go:60-64`
documents it: *"separate from Provider so adding it never forces every provider
(or test stub) to implement it. Services reach it with a type assertion."*
`CredentialActivator` follows it verbatim.

The honest counter-argument: `CLAUDE.md` §2 warns against abstractions for
single-use code, and this interface has exactly one implementer. It is accepted
because the alternative pushes provider knowledge upward, which is the more
expensive mistake. Service behaviour when a provider does **not** implement it:
fall back to `Activate`, and reject a `--profile` that is not the primary rather
than silently activating the wrong one.

### D2 — Where the `kuberoutectl.io/credential` system label is set

System labels are provider-owned (`buildTarget` sets provider/source/platform/
health today). So the label is set in each provider's `buildTarget`, **all four**,
not AWS alone. A label that only one provider populates is a trap: a selector
like `--selector kuberoutectl.io/credential=x` would silently never match an
Azure or GCP target, and the operator would read that as "no match" rather than
"not implemented". Four one-line additions.

### D3 — Reachability is discovered, not guessed; and how far that reaches

The motivating environment has **no pattern**: profile A reaches some EKS
clusters, profile B reaches others, and nothing in the naming or tagging predicts
which. That does not need a use-time failover, because `discoverClusters`
(`aws.go:130-152`) already resolves it empirically, at two levels:

```go
listOut, _, err := p.runner.Run(ctx, awsBin, "eks", "list-clusters", "--profile", profile, ...)
if err != nil { return nil }      // profile cannot list at all → no targets
...
    descOut, _, derr := p.runner.Run(ctx, awsBin, "eks", "describe-cluster", "--profile", profile, ...)
    if derr != nil { continue }   // profile cannot describe THIS cluster → dropped
```

`eks:ListClusters` is account-wide, but `eks:DescribeCluster` is evaluated **per
resource**. So after the fold, a target's `CredentialIDs` is exactly the set of
profiles that can describe that cluster — computed during sync, not by trial and
error against a live cluster. `target use <ref>` with no flag therefore picks a
working profile in the no-pattern case, and `--profile` stays what it should be:
a manual override.

**Where this stops — and this is where the motivating environment actually
lives.** `describe-cluster` is an **IAM** permission. Operating inside the
cluster additionally requires an EKS **access entry** (formerly the `aws-auth`
ConfigMap), which discovery does not read. The operator has confirmed their
profiles differ at the access-entry layer, not only at IAM. So the common case
for them is:

```
profile A: describe OK  →  in CredentialIDs  →  update-kubeconfig OK  →  kubectl: Forbidden
profile B: describe OK  →  in CredentialIDs  →  update-kubeconfig OK  →  kubectl: works
```

Both land in `CredentialIDs`, the primary is chosen by health and alphabetical
tie-break, and **nothing distinguishes them**. `target use <ref>` with no flag is
then a coin flip, with no signal that it landed wrong.

**What this plan delivers for that environment, stated plainly:**

| | |
|---|---|
| Duplicate, mutually-unreachable targets | **fixed** — independent of the access-entry question |
| `--profile` manual override | **works** — once you know cluster X needs B, you can pin it |
| Picking the right profile automatically | **no** — the primary is a guess |
| An agent choosing autonomously | **no** — it would guess too |

This is also why **failover at activation would be worthless**: `aws eks
update-kubeconfig` needs only `describe`, which already succeeded during
discovery, so it does not fail in the interesting case. The failure surfaces at
the first `kubectl` call.

Closing the gap needs a real operability signal, and there are two ways in. They
are **deferred to their own spec** — noted here so that spec starts from the
better one rather than the obvious one:

- **`kubectl auth can-i` probe** — the obvious approach. Correct for every
  cluster, but puts `kubectl` on kuberoutectl's path and needs a kubeconfig entry
  per (cluster, profile) combination before it can even ask.
- **`aws eks list-access-entries` — better.** Stays inside the AWS CLI this
  adapter already drives, needs no kubeconfig, and discovery already holds each
  profile's ARN from `sts get-caller-identity`. Two known traps for that spec:
  an SSO profile's caller identity is an `assumed-role` ARN while the access
  entry is registered against the **IAM role** ARN, so comparison needs
  normalization, not string equality; and clusters whose `authenticationMode` is
  `CONFIG_MAP` have no access entries at all, so the mode must be read from
  `describe-cluster` (`awsCluster` in `parse.go:17` does not parse `accessConfig`
  today) and that case handled as unknown rather than as "no access".

What this plan does add for that boundary is **visibility**: today's `continue`
on a describe failure is silent, and `discoverClusters` does not even receive the
`Progress`. Task 4b threads it through, so each `sync aws` reports which profile
was denied on which cluster — turning an undocumented access map into sync
output. That serves the no-pattern environment directly and costs a parameter.

### D4 — The fold copies a winner; it does not patch a base

Found in Gate 3.5 review, not in the original draft. Today `Target.Health` and
`SystemLabels[domain.LabelHealth]` **cannot** diverge: `buildTarget`
(`build.go:67-79`) writes both from the same value in one pass, and nothing
mutates either afterwards. The fold is the first code that could break that
invariant.

It matters because `SelectionLabels()` (`internal/domain/target.go:87-94`)
exposes health under **two different keys**: the bare alias `health`, computed
from `t.Health`, and the qualified `kuberoutectl.io/health`, copied verbatim out
of `SystemLabels`. So a fold that patches the scalar fields onto whichever
candidate it happened to iterate first leaves the qualified key stale:
`--selector health=valid` matches while `--selector kuberoutectl.io/health=valid`
does not, for the same target. Once D2 lands, `kuberoutectl.io/credential` has
exactly the same exposure.

**Rule**: the fold selects a winner and returns **that candidate's own struct**,
adding only `CredentialIDs`. It never assembles a target from parts. Every
provider-owned field — including maps — is therefore internally consistent by
construction, exactly as it is today.

This is a distinct failure class from a leaf field simply being unset: the value
is present, well-formed, and wrong. Field-by-field assertions miss it, so task 3
compares `SystemLabels` as a whole map against the primary candidate's.

### D5 — No `Metadata["profiles"]`; display names come from the join

The first draft stored a comma-joined profile list on the target for the
`PROFILES` column. Dropped after Gate 3.5 pointed out it contradicts this plan's
own principle: the spec is explicit that per-credential **health** must be a live
join against `snapshot.Credentials` — *"no new struct, no second copy of health
to drift"* — and then allowed the profile names to be a frozen string anyway.

Task 12 already builds exactly that join for `target inspect`. Extending it to a
list form (`ListWithCredentials`, one snapshot load for the whole page) lets
`target list` render the same data from the same source. The denormalized field,
its drift risk, and the test guarding that risk all disappear together, so this
removes about as much as it adds.

Credential **names** are the join's job because `Target` holds only
`CredentialID`s (`aws:ops`), and turning those into `ops` by stripping a prefix
in the CLI would put provider-specific string surgery in the presentation layer.
`Credential.Name` already carries the profile (`build.go:31`).

---

## Components

| Component | Type | Purpose |
|-----------|------|---------|
| `Target.CredentialIDs` | domain field | every credential reaching a target, primary first |
| `Selection.CredentialID` | domain field | which access path the selection was activated through |
| `domain.LabelCredential` | domain constant | system label key for the primary credential name |
| `foldTargetsByID` | pure function (aws) | collapses per-(profile, cluster) targets into one per cluster |
| `credentialRank` | pure function (aws) | total order over `AccessHealth` for primary selection |
| `providers.CredentialActivator` | optional interface | activate through a named credential (see D1) |
| `aws.Provider.ActivateAs` | provider method | `aws eks update-kubeconfig --profile <chosen>` |
| `UseTargetOptions` / `UseTargetResult` | service types | carry the credential choice in and the resolved credential out |
| `TargetService.ResolveWithCredentials` / `ListWithCredentials` | service methods | target(s) + their credentials, joined from the snapshot — the single source of credential names for display (D5) |

## New files

None. Every change lands in an existing file — the feature is additive to
structures that already exist.

## Files to change

| File | What changes | Why |
|------|-------------|-----|
| `internal/domain/target.go` | `+ CredentialIDs []CredentialID` | spec Data model |
| `internal/domain/snapshot.go` | `+ Selection.CredentialID` | persist the choice |
| `internal/domain/labels.go` | `+ LabelCredential` | selector support |
| `internal/providers/aws/build.go` | `foldTargetsByID`, `credentialRank`, populate `CredentialIDs` | the fix itself |
| `internal/providers/aws/aws.go` | call the fold before sorting `res.Targets`; `discoverClusters` gains a `Progress` param and reports per-cluster access denials | wire it; D3 |
| `internal/providers/aws/activate.go` | `+ ActivateAs` | D1 |
| `internal/providers/{azure,gcp,kubeconfig}/build.go` | set `LabelCredential` | D2 |
| `internal/providers/provider.go` | `+ CredentialActivator` | D1 |
| `internal/services/selection.go` | credential resolution, fallback, options/result types, status reports credential | spec Selection |
| `internal/services/inventory.go` | `ResolveWithCredentials` | inspect breakdown |
| `internal/cli/target.go` | `--profile` flag, `PROFILES` column, inspect breakdown | spec CLI |
| `internal/cli/current.go` | `Profile` line | spec CLI |
| `internal/mcpserver/tools_write.go` | `UseTargetInput.Profile` | spec MCP |
| `scripts/e2e.sh` | second AWS profile over one account | spec Testing ladder |
| `internal/providers/aws/testdata/` | identity + describe fixtures for the second profile | provider tests |
| `CHANGELOG.md`, `docs/` | document the flag and the fold | release hygiene |

---

## Tasks

### Phase 1 — Domain (no behaviour change)

| # | Task | Files | Test |
|---|------|-------|------|
| 1 | Add `Target.CredentialIDs`, `Selection.CredentialID`, `LabelCredential`, all `omitempty` | `internal/domain/target.go`, `snapshot.go`, `labels.go` | JSON round-trip **with and without** the new keys — the without case is edge case 8's foundation |

Nothing reads the fields yet; the build stays green on its own.

### Phase 2 — AWS fold (depends on 1)

| # | Task | Files | Test |
|---|------|-------|------|
| 2 | `credentialRank` — total order `valid > expiring > static > expired > unknown` | `internal/providers/aws/build.go` | table test over every `AccessHealth` value, including one not in the list (must sort last, not panic) |
| 3 | `foldTargetsByID` — group by `Target.ID`; primary = best rank, then alphabetical profile. **The primary's own pre-fold `Target` is the base struct**, copied whole; the fold adds `CredentialIDs` and nothing else. It must never patch scalar fields onto a different candidate — see D4 | `internal/providers/aws/build.go` | edge cases 1, 3, 4, 9 as pure-function tests. Plus D4's assertion: post-fold `SystemLabels` must be **identical** to the primary candidate's, compared as a whole map, not field by field |
| 4 | Call the fold in `Discover` before the existing `sort.Slice`; adjust the `discovered N cluster(s)` progress line to report folded count | `internal/providers/aws/aws.go` | FakeRunner test: two profiles, one account, one shared cluster → exactly one target |
| 4b | Thread `Progress` into `discoverClusters` and `Step` on each per-cluster `describe-cluster` failure, naming the profile and the cluster | `internal/providers/aws/aws.go` | FakeRunner test: a profile denied on one cluster emits a step naming both, and still yields no target for it |
| 5 | Fixtures for the second profile (`identity-ops.json`, reuse the existing describe fixtures) | `internal/providers/aws/testdata/*.json`, `aws_test.go` | edge cases 2 and 11 — different regions produce no fold; an STS-failed profile appears in no `CredentialIDs` |
| 6 | Set `LabelCredential` in AWS + Azure `buildTarget` | `internal/providers/aws/build.go`, `internal/providers/azure/build.go` | assert the label is present and equals the primary credential's name |
| 7 | Same for GCP + kubeconfig | `internal/providers/gcp/build.go`, `internal/providers/kubeconfig/build.go` | same assertion per provider |

Tasks 6–7 are split only to keep each task ≤ 3 files (D2 requires all four).

### Phase 3 — Services (depends on 1, 2)

| # | Task | Files | Test |
|---|------|-------|------|
| 8 | `providers.CredentialActivator` interface + AWS `ActivateAs` | `internal/providers/provider.go`, `internal/providers/aws/activate.go` | FakeRunner asserts `--profile ops` reaches the command line |
| 9 | `UseTargetOptions{Activate, CredentialName}` / `UseTargetResult{Target, Credential}`; replace the `UseTarget(ctx, ref, activate)` signature and update both call sites | `internal/services/selection.go` | existing selection tests migrated; behaviour with empty options must be byte-identical to today |
| 10 | Credential resolution: explicit name → remembered → primary; unknown name and single-credential targets rejected **before** any external CLI runs | `internal/services/selection.go` | edge cases 5, 6, 7 |
| 11 | `SelectionStatus` carries the selected credential and marks it absent when a resync dropped it | `internal/services/selection.go` | edge case 5's `current` half |
| 12 | `TargetService.ResolveWithCredentials` (one target) and `ListWithCredentials` (many) — targets plus their credentials, joined from the snapshot, primary first. This join is the **only** source of credential names for display; no denormalized copy exists — see D5 | `internal/services/inventory.go` | join is order-stable; a target with no `CredentialIDs` yields its single `CredentialID`; the snapshot is loaded once for the whole list, not per target |

Task 9 is a **breaking signature change** inside `internal/`. Two call sites
(`internal/cli/target.go`, `internal/mcpserver/tools_write.go`) plus tests. No
external consumers — `internal/` is not importable outside the module.

### Phase 4 — CLI (depends on 3)

| # | Task | Files | Test |
|---|------|-------|------|
| 13 | `--profile` flag on `target use`; the success line reports the profile used **and how it was chosen** — explicit flag, remembered from a previous use, or an unprompted default. The default case must say so and point at `--profile`, because D3 establishes that landing on the primary carries no signal that it might be wrong | `internal/cli/target.go` | golden output for all three cases; the default case must not be worded identically to the explicit one |
| 14 | `PROFILES` column on `target list`, rendered from task 12's join, shown only when some target has more than one — mirroring the existing `HIDDEN`-column rule at `target.go:71-79` | `internal/cli/target.go` | column absent for a single-profile fleet; present and correct otherwise |
| 15 | Per-credential breakdown on `target inspect`, primary marked | `internal/cli/target.go` | golden output |
| 16 | `Profile` line on `current`, marked when the profile was an unprompted default rather than a choice (same reasoning as task 13), plus the "no longer present" note | `internal/cli/current.go` | golden output for all three states |

Tasks 13–15 touch the same file and run sequentially.

### Phase 5 — MCP (depends on 3)

| # | Task | Files | Test |
|---|------|-------|------|
| 17 | `UseTargetInput.Profile` + `UseTargetOutput.Profile`; jsonschema description written to match the existing tag style | `internal/mcpserver/tools_write.go` | MCP path and CLI path produce the **same** persisted selection for the same input |

### Phase 6 — Verify and document (depends on all)

| # | Task | Files | Test |
|---|------|-------|------|
| 18 | e2e fake `aws` gains an `ops` profile over `prod-sso`'s account seeing the same cluster; assert one target, two profiles, and `target use --profile ops` | `scripts/e2e.sh` | the ladder's top rung, against the shipped binary |
| 19 | Pre-upgrade snapshot fixture, hand-written in the old shape, loaded read-only | `internal/services/*_test.go`, new `testdata` snapshot | edge case 8 — **must not** be regenerated by the new code or it proves nothing |
| 20 | Kubeconfig overlay suppression still collapses the AWS cluster's duplicate context | `internal/services/dedup_test.go` | edge case 10 |
| 21 | `CHANGELOG.md` + docs for `--profile` and the fold | `CHANGELOG.md`, `docs/` | docs build |

### Parallel vs sequential

| Parallel group | Tasks | Why |
|---------------|-------|-----|
| A | 2, 3 | pure functions, independent of each other until 3 calls 2 — write 2 first, then 3 |
| A' | 4b | touches `discoverClusters`, which the fold does not; independent of 2–4 despite sharing `aws.go` |
| B | 6, 7 | different provider packages, no shared file |
| C | 8, 12 | different packages, no shared symbol |
| D | 19, 20 | independent test files |

| Sequential | Depends on | Why |
|-----------|-----------|-----|
| 3 | 2 | `foldTargetsByID` ranks with `credentialRank` |
| 4 | 3 | wires the fold |
| 5 | 4 | fixtures exercise the wired path |
| 9 | 8 | activation must exist before selection can route to it |
| 10, 11 | 9 | build on the new options/result types |
| 13–16 | 10, 11 | render what the service resolves |
| 17 | 10 | same service entry point |
| 18 | 13, 17 | e2e drives the shipped binary |

---

## Testing plan

Every row ties to a spec requirement or numbered edge case.

| Level | Package | Covers |
|-------|---------|--------|
| Domain / persistence | `internal/domain` | JSON round-trip with and without the new fields (edge case 8) |
| Provider, pure | `internal/providers/aws` | fold, ranking, tie-breaks, determinism (edge cases 1, 3, 4, 9) |
| Provider, FakeRunner | `internal/providers/aws` | two profiles one account; region split; STS-failed profile (edge cases 2, 11); `ActivateAs` passes `--profile` |
| Provider, no-pattern case | `internal/providers/aws` | profile A describes cluster X but not Y, profile B the reverse → each target's `CredentialIDs` holds only the profile that can describe it, and each denial is reported through `Progress` (D3, task 4b) |
| Provider, all four | `internal/providers/*` | `LabelCredential` present and correct (D2) |
| Service | `internal/services` | credential resolution order, rejections before any exec, vanished credential, inspect join, overlay suppression (edge cases 5, 6, 7, 10) |
| Service, migration | `internal/services` | hand-written pre-upgrade snapshot loads, lists, activates unchanged (edge case 8) |
| CLI | `internal/cli` | flag parsing, three success messages, conditional column, inspect breakdown, `current` |
| MCP | `internal/mcpserver` | `profile` honoured; CLI and MCP converge on the same persisted selection |
| e2e | `scripts/e2e.sh` | the whole flow against the shipped binary |

**Ladder**: `go test ./... -count=1` → `make check` → `bash scripts/e2e.sh`.
`-count=1` matters here — task 19 reads a fixture from disk at runtime, and a
cached PASS would prove nothing about the file's contents.

**Stated gap, to repeat in the PR**: no test authenticates two real AWS profiles
against a real account. The fold, the flag, and the persistence are
fixture-driven. What stays unproven is that a live `aws configure list-profiles`
plus two live STS identities behave as the fixtures model them.

---

## Risks

| Risk | Mitigation |
|------|-----------|
| Task 9 changes a service signature used by both `cli` and `mcpserver` | Do it in one commit with both call sites; empty options must be behaviourally identical to today, asserted by the migrated tests |
| The fold silently drops a target if grouping is wrong | Task 4's test asserts an exact target count, not just "contains" |
| The fold produces a target whose `SystemLabels` belong to a different candidate than its scalar fields — a value that is present, well-formed and wrong | D4: the fold returns the winner's own struct rather than assembling one. Task 3 asserts `SystemLabels` as a whole map, since field-by-field checks cannot see this |
| ~~`Metadata["profiles"]` drifting from `CredentialIDs`~~ | Removed rather than mitigated — D5 deletes the denormalized field |
| Pre-existing: `target inspect` iterates `SystemLabels` as a map, so label order is nondeterministic | **Not fixed here** — out of scope per `CLAUDE.md` §3. Noted so the builder does not mistake it for damage caused by task 15, and so the inspect golden test tolerates label order |

---

## Shipping

**Two PRs, not one.** Phases 1–2 (tasks 1–7) are complete and shippable alone:
they fix the spec's Problem section — duplicate, silently unreachable targets —
with no change to the CLI, MCP, or selection surface. Phases 3–6 add the
`--profile` UX on top. Splitting keeps task 9's service-signature change, the one
item flagged in Risks, out of the bug-fix PR.

**The spec's nice-to-have is dropped, not deferred.** `target inspect -o json`
currently renders the bare `domain.Target` (`internal/cli/target.go:125`).
Wrapping it as `{target, credentials}` to expose the per-credential array would
**break that command's JSON shape** for anything already parsing it. The new
`credential_ids` field is additive and ships; consumers needing per-credential
health join against `credential list` / `list_credentials`, which is what the MCP
path already does. Revisit only alongside a deliberate output-shape change.

## Gate 2

**Architecture**
- [x] Follows the project's existing architecture — `domain ← providers/services ← cli/mcpserver`; the skill's Controller/Manager/Repository template is explicitly rejected as inapplicable, with the real layering substituted.
- [x] Each layer only calls the layer below — verified by D1, which exists precisely to keep AWS's `profile` metadata key out of `internal/services`.
- [x] Components are in the right directories — no new files; every change lands in the package that already owns that concern.

**Task breakdown**
- [x] All files to change are listed — 16 rows, including fixtures, e2e, and docs.
- [x] All new files listed — there are none, stated explicitly rather than left blank.
- [x] Each task is small — every task is ≤ 3 files; tasks 6/7 exist as a pair solely to honour that limit. Task **4b** is numbered as an insertion rather than renumbering 5–21, which would have cascaded through both dependency tables and every edge-case mapping below; it is a full task, not a sub-step of 4.
- [x] Dependencies between tasks are clear — the sequential table names the reason for each edge.
- [x] Parallel vs sequential marked — four parallel groups, eight ordering constraints.

**Testing**
- [x] Data-layer tests planned — domain round-trip plus the pre-upgrade snapshot fixture.
- [x] Business-logic tests planned — fold, ranking, credential resolution, rejection paths.
- [x] API/integration tests planned — CLI golden output, MCP parity, e2e against the shipped binary.
- [x] UI tests — not applicable; this CLI's equivalent is the golden-output tests, which are planned.
- [x] Every spec edge case is mapped — 1, 3, 4, 9 → task 3; 2, 11 → task 5; 5, 6, 7 → tasks 10–11; 8 → task 19; 10 → task 20.

**Closed since Gate 1**: the reachability-selector question. The motivating use
case is an ops agent picking a cluster it can operate on, and D3 shows the fold
already answers that per cluster without any selector — the agent iterates the
inventory and reads `Health`. No selector grammar change is needed, and the
single-valued label is not a limitation for this use case.

**Deferred to its own spec, and it is not optional for the motivating
environment**: distinguishing "IAM lets me describe this cluster" from "I can
actually operate in it". The operator's profiles differ at the EKS access-entry
layer, which this plan does not read — so automatic profile selection remains a
guess for them, and only the `--profile` override is reliable. D3 sizes exactly
what is and is not delivered, and points that spec at
`aws eks list-access-entries` rather than a `kubectl auth can-i` probe.

**Carried forward from Gate 1, still unresolved by design**: the
`kuberoutectl.io/credential` label is single-valued and holds the primary only,
so a selector can express "ops is primary" but not "reachable via ops". If
filtering by reachability is the real need, it changes the selector grammar and
belongs in its own spec — say so before `/spartan:build` rather than after.
