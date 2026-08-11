# Plan: improve CodeQL analysis

**Spec**: `.planning/specs/improve-codeql-analysis.md`
**Epic**: none
**Created**: 2026-07-29
**Status**: phases 1 and 2 done; phase 3 blocked on the fork question (task 8)
**Target release**: 1.2.0

## Stack note

Go CLI — but nothing here is Go. This feature is **one workflow file and one
repository ruleset**, so the Controller → Manager → Repository template does not
apply and is deliberately not used. There is no data layer, no business logic and
no UI; the "testing plan" below is correspondingly unusual and says so rather
than inventing layers to fill a template.

The consequence worth stating up front: **the ruleset is repository state, not
code**. It is not in the diff, not reviewable in a PR, and not covered by the
verification ladder. A plan that treats it like a file will produce a PR that
looks complete and changes nothing.

---

## Architecture decisions

### D1 — Edit the existing ruleset, do not add a second

There is exactly one ruleset, `default` (id `18918362`), already targeting
`refs/heads/main`, `refs/heads/development` and `~DEFAULT_BRANCH`, with rules
`deletion`, `non_fast_forward`, `copilot_code_review`.

A second ruleset would also apply — rulesets are additive, not
last-one-wins — so two rulesets over the same refs means two places to look when
a merge is blocked and nobody knows why. Add a `required_status_checks` rule to
the one that exists.

### D2 — Pin each required check to the app that produces it

A required check matches by **name**. Any GitHub App with `checks: write` can
post a check run called `verify`, and a rule that matches on name alone would
accept it. The API takes an `integration_id` alongside the context, which pins
it to the producer:

| Context | App | `integration_id` |
|---|---|---|
| `verify` | `github-actions` | `15368` |
| `goreleaser-check` | `github-actions` | `15368` |
| `Analyze (go)` | `github-actions` | `15368` |
| `Analyze (actions)` | `github-actions` | `15368` |
| `CodeQL` *(phase 3)* | `github-advanced-security` | `57789` |

All five read from the API on a live PR (`commits/{sha}/check-runs`), not from
the workflow files — matrix jobs report as `Analyze (go)`, never as the job's
`name:` field, and that mismatch is the usual reason a required check never
arrives.

### D3 — Phases are separate PRs, not commits in one

Spec requirement, restated because it is the plan's whole shape: each phase has
exactly one way to fail.

- Phase 1 changes no files at all. Its risk is "the repository becomes
  unmergeable", and it is verified by opening a throwaway PR.
- Phase 2 changes one file. Its risk is "the deeper suite reports a backlog".
- Phase 3 is one more line in the ruleset, and is gated on phase 2's findings
  reaching zero *and* on the fork question being settled.

Folding them together means a failure has three candidate causes.

*Corrected 2026-07-30:* an earlier revision made the bypass decision (task 0) a
**blocking** prerequisite of phase 1, reasoning that deciding it afterwards
meant "a second edit made under whatever pressure prompted it". That is not
true — a ruleset can be edited on any calm day, and adding `bypass_actors` later
is neither harder nor more constrained than adding it now. There is no
irreversibility and no ordering constraint, so the dependency was rigour-shaped
rather than real. It survived three rounds of review, which is worth recording:
wrong facts get caught because they are checkable, and over-tightened reasoning
reads as care.

### D4 — `strict` off, and stated rather than defaulted

`strict_required_status_checks_policy: false` — do not require branches to be up
to date before merging. With one maintainer and squash merges it turns every
merge into a rebase, to prevent a semantic-conflict race that this repository's
test suite would catch on the next PR anyway.

---

## Components

| Component | Type | Purpose |
|---|---|---|
| `required_status_checks` rule | repository ruleset | the gate itself |
| `codeql.yml` | GitHub Actions workflow | the analysis, deepened |
| a throwaway PR | verification | the only way to observe the gate |

## Files to change

| File | What changes | Why |
|---|---|---|
| `.github/workflows/codeql.yml` | `queries: security-extended`; delete the `Run manual build steps` step and the template comment blocks; `actions/setup-go@v4` → `@v5` | spec requirements 5, 6, 7 |
| `CLAUDE.md` | note that required checks exist and a job rename needs a ruleset update | spec nice-to-have; edge case 2 |

**No new files.** A `codeql-config.yml` is spec nice-to-have and has nothing to
hold while there is one suite and no exclusions.

**Not a file**: the ruleset. Applied through the API, recorded here.

---

## Tasks

### Phase 1 — gate the checks that already pass (no code)

| # | Task | Verifies |
|---|------|----------|
| 0 | Note the bypass posture: `bypass_actors` is `[]`, so once checks are required the only way to merge a red PR is disabling the ruleset — which drops `deletion` and `non_fast_forward` with it. Decide whether to add a bypass. **Not blocking** | spec req 4 |
| 1 | Add `required_status_checks` to ruleset `18918362` with the four contexts from D2, each pinned to `integration_id` 15368, `strict: false` | spec req 1, 5 |
| 2 | Open a throwaway PR touching only `README.md`; confirm it is **mergeable** and does **not** wait on `reproducible-build` | spec req 2, edge case 1 — the failure mode that makes the repo unmergeable |
| 3 | On the same PR, push a commit that breaks a test; confirm the merge button is **blocked**; revert and close | spec goal 1 — the gate has never been observed to work until this is done |

Phase 1 is not complete until task 3 has been *seen*, not inferred — and the
observation has to leave a trace. "Seen, not inferred" with no artifact is a
checkbox anyone can tick from memory weeks later, including the person who never
did it.

**Record it here.** Fill this table before marking phase 1 complete; a blank cell
is the signal, and it is the only one this plan has:

Every row records a **commit SHA or a timestamp**, never a yes/no. A yes/no is
satisfiable by typing it; a SHA is a handle anyone can re-derive the answer from,
a year later, without asking whoever filled it in.

| Evidence | Value | How anyone re-checks it |
|---|---|---|
| Throwaway PR (tasks 2, 3) | [#129](https://github.com/ymedlop/kuberoutectl/pull/129), closed unmerged | open it |
| Task 2 — the clean commit | `679439b7ef1f675f7637983790b582ba559e2afc` | `gh api repos/ymedlop/kuberoutectl/commits/<SHA>/check-runs --jq '[.check_runs[].name]'` must **not** contain `reproducible-build` |
| Task 3 — the breaking commit | `62d924bb62b1fc61d0093bc5f2c6c829537bd29d` | the same call must show `verify` with `conclusion: failure`, which is what the merge was blocked on |
| Ruleset `updated_at` after task 1 | `2026-07-31T14:17:59.776Z` | must differ from the baseline below |

Observed on 2026-07-31, in this order:

- On `679439b` — a README-only diff — the check set was exactly
  `Analyze (actions)`, `Analyze (go)`, `CodeQL`, `goreleaser-check`, `verify`.
  `reproducible-build` is absent, so the path filter holds and requiring it
  would have hung this PR forever. `mergeStateStatus` moved `BLOCKED → CLEAN`
  as the checks landed.
- On `62d924b` — `AccessVerdict.SelectorValue` inverted so `true`/`false` swap
  — `verify` reported `failure` and `mergeStateStatus` stayed `BLOCKED` with
  **zero checks pending**. Pending-zero is the load-bearing part: it separates
  "blocked because a check failed" from "blocked because something never
  reported", which look identical in the UI.

Note `mergeable` stays `MERGEABLE` in both cases. That field is git-level
(no conflicts) and says nothing about the ruleset; `mergeStateStatus` is the one
that moves. Reading the wrong field would have shown the gate passing while it
did nothing.

Task 0 resolved by the repository owner: `bypass_actors` stays `[]`. The escape
hatch for a red PR remains disabling the ruleset, which drops `deletion` and
`non_fast_forward` with it until re-enabled.

One difference from what this plan proposed: `do_not_enforce_on_create` is
`true`, not `false`. It only governs branch/tag *creation*, and both target
branches already exist, so it changes nothing here — recorded so the drift is
not mistaken later for a mis-set field.

Baseline, read 2026-07-30: `updated_at` is **`2026-07-15T18:52:07.376Z`** — the
ruleset has not been touched in two weeks. Recording the baseline is what makes
that row falsifiable: a timestamp on its own can be filled in with anything,
while "differs from 2026-07-15T18:52:07.376Z" cannot be satisfied without the
write having happened.

Prose describing an audit trail is not an audit trail. An earlier revision of
this plan said "the PR URL is the evidence" without saying where it goes, which
left the requirement satisfiable by agreeing with it.

### Phase 2 — deepen the analysis (depends on nothing; may run before or after 1)

| # | Task | Files |
|---|------|-------|
| 4 | `queries: security-extended` | `.github/workflows/codeql.yml` |
| 5 | Delete the `Run manual build steps` step and the template comment blocks; keep comments that record a decision this repo made | `.github/workflows/codeql.yml` |
| 6 | `actions/setup-go@v4` → `@v5` | `.github/workflows/codeql.yml` |
| 7 | Merge, let the scheduled + PR runs report, triage every finding to zero (fix or dismiss with a reason) | — |

Tasks 4–6 are one commit: same file, one logical change, and splitting them would
produce three PRs against a file whose diff is easier to read whole.

**Phase 2 evidence** (PR #131, observed 2026-07-31):

| Evidence | Value | How anyone re-checks it |
|---|---|---|
| Both legs pass under the deeper suite | `Analyze (go)` and `Analyze (actions)` both `success` | `gh api repos/ymedlop/kuberoutectl/commits/<SHA>/check-runs` |
| Task 7 — the backlog | **0 open alerts**, on `refs/pull/131/merge` and repo-wide | `gh api 'repos/ymedlop/kuberoutectl/code-scanning/alerts?state=open'` |

Two things this settled that the plan had left open:

- **The `actions` language does ship a `security-extended` suite.** `queries:`
  applies to both matrix legs, and it was not confirmed beforehand that the
  newer `actions` pack defines the suite. It does; `Analyze (actions)` passed.
- **Task 7 is satisfied trivially.** `security-extended` reported nothing to
  triage. Worth stating plainly rather than as a win: this is a small Go CLI
  that shells out to provider binaries through `execx` instead of parsing
  untrusted input, and its own rule forbids network calls outside
  `internal/cli`. That is close to the shape a security suite has least to say
  about — the empty result describes the codebase, not the suite's depth.

Phase 3 is therefore unblocked on findings, and still blocked on task 8: whether
a **fork** PR produces the `CodeQL` check. Every PR observed so far came from a
branch in this repository, which says nothing about forks — the permissions
differ, and that is the whole question.

### Phase 3 — gate on CodeQL (depends on 1 and 7)

| # | Task | Verifies |
|---|------|----------|
| 8 | Establish whether a fork PR produces the `CodeQL` check in this public repo | spec edge case 4 — **blocking**: if it does not, phase 3 is rejected and the reason recorded in the spec |
| 9 | Add `CodeQL` (`integration_id` 57789) to the required contexts | spec req 9 |
| 10 | `CLAUDE.md`: record that required checks exist and that renaming a job needs a ruleset update | spec nice-to-have, edge case 2 |

### Parallel vs sequential

| Parallel | Tasks | Why |
|---|---|---|
| A | Phase 1 (1–3) and Phase 2 (4–6) | different surfaces entirely — one is repository state, the other a file |

| Sequential | Depends on | Why |
|---|---|---|
| 2 | 1 | run before the rule exists, "it does not wait on `reproducible-build`" is true because **nothing** is required yet. The test passes for the wrong reason and gets ticked as having verified requirement 2 |
| 3 | 1, 2 | there is nothing to observe until the rule exists |
| 7 | 4 | the backlog cannot be triaged before the suite that reports it |
| 9 | 7, 8 | gating on a suite with an untriaged backlog blocks every PR; gating on a check forks cannot produce blocks every contribution |

---

## Testing plan

There is no code under test. What follows is the honest mapping instead of the
template's layers:

| Level | Exists? | What stands in |
|---|---|---|
| Data layer | no | — |
| Business logic | no | — |
| API / integration | **yes, manual** | tasks 2, 3 and 8 — a live PR is the only instrument |
| UI | no | — |

| Spec edge case | Covered by |
|---|---|
| 1 — a required check that never runs | task 2 |
| 2 — a renamed job silently un-required | task 10 (documented); **not testable** — see risks |
| 3 — `security-extended` reports a backlog | task 7, and the phase ordering itself |
| 4 — fork PRs | task 8, blocking phase 3 |
| 5 — maintainer needs to merge red | admin bypass, deliberate, nothing to test |
| 6 — a dismissed finding recurs | expected behaviour, nothing to test |
| 7 — scheduled run finds something between PRs | accepted, nothing to test |

**The verification ladder does not apply to phase 1 or 3.** `make check`,
`scripts/e2e.sh` and the rest read the repository, and the ruleset is not in it.
Saying "all five rungs pass" about a ruleset change would be true and irrelevant.

Phase 2 runs the ladder, but **no rung can be affected** by an edit confined to
`codeql.yml`: `make docs-reference` renders the Cobra command tree, which a
workflow file does not touch. They pass regardless, and reporting them is
evidence of nothing.

---

## Risks

| Risk | Mitigation |
|---|---|
| A required context name does not match what GitHub posts, and every PR hangs | Names and app ids read from `commits/{sha}/check-runs` on a live PR, not from workflow files; task 3 observes it before it matters |
| `reproducible-build` gets added to the list by someone later | Spec requirement 2 states it; task 2 would catch it next time it happens |
| A job rename silently drops its requirement | **Unmitigated.** Nothing warns, and there is no test — the ruleset lives outside the repo. Task 10 documents it; that is the whole defence, and it is weak |
| `security-extended` reports enough to be ignored rather than triaged | Phase ordering: it does not gate anything until it is at zero, so an untriaged backlog is visible but not obstructive |
| Fork PRs cannot produce `CodeQL` | Task 8 blocks phase 3 on establishing it |
| Phase 1 is "done" on paper while the ruleset was never applied | The evidence table above must be filled before phase 1 is complete. Blank cells are the check; "a PR that merges anyway is the failure" was the previous mitigation and relied on someone noticing a negative |
| No bypass exists, so a wrong context name locks the repository with no escape but disabling the ruleset | Task 2 catches a wrong name on a throwaway PR before it matters. If it is ever needed, disabling the ruleset is a switch rather than a scalpel — it drops the deletion and force-push rules too, on both branches, until someone remembers to re-enable it |

---

## Gate 2

**Architecture**
- [x] Follows the repo's patterns where they exist, and states plainly where the
      template does not apply rather than inventing layers.
- [x] One ruleset rather than two, with the reason (rulesets are additive).
- [x] Required checks pinned to their producing app, not matched on name alone.

**Task breakdown**
- [x] Every file to change listed — one workflow, one doc.
- [x] No new files, stated with the reason.
- [x] Tasks small; 4–6 grouped deliberately as one commit on one file.
- [x] Dependencies explicit, with the reason on each edge.
- [x] Parallel vs sequential marked.

**Testing**
- [x] Every spec edge case mapped, including three that are correctly *not*
      testable and say so.
- [x] The one blocking unknown (fork behaviour) is a task that gates phase 3 —
      and it is blocking for a reason task 0 was not: its harm is invisible and
      lands on contributors, where an absent bypass is visible and lands on the
      person who chose it.
- [x] Stated that the verification ladder does not cover phases 1 and 3, rather
      than reporting it green and implying it means something.

**Called out rather than assumed**
- The check names and app ids are read from the API, because the workflow's
  `name:` fields do not match what a matrix job reports — the most likely way to
  produce a permanently unmergeable repository.
- Edge case 2 has no mitigation beyond documentation, and the plan says so
  instead of implying the doc solves it.

**Gate 2: PASSED**
