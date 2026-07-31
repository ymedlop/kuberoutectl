# Spec: improve CodeQL analysis

**Created**: 2026-07-29
**Status**: phase 1 done (ruleset gated + verified); phases 2 and 3 open
**Author**: Yeray Medina López
**Epic**: none
**Target release**: 1.2.0

---

## Problem

`.github/workflows/codeql.yml` is the stock GitHub template with two lines
changed. It runs the **default query suite** — the narrowest of the three — and
the `queries:` line that would widen it is still commented out. 42 of its 104
lines are instructional comments, and it carries a
`Run manual build steps` step that can only ever fire as a confusing `exit 1`,
because `build-mode` is never `manual`.

It is always green, and nobody has reason to believe that means anything.

Separately, and more seriously: **nothing gates a merge.** The active ruleset
covers `main` and `development` and enforces `deletion`, `non_fast_forward` and
`copilot_code_review`. Its `required_status_checks` list is **empty**. Every PR
in this repository can be merged red, including the ones where CI was watched
turning green first. `CLAUDE.md` calls `development` "branch-protected", which is
true only in the sense that it cannot be deleted or force-pushed.

A deeper analysis that gates nothing is more text in a tab. A gate on the checks
that already exist is value available today.

## Goal

Merges are gated on the checks that already pass, and CodeQL looks harder than
the default suite before it is asked to gate anything.

Verifiable:

1. A PR with a failing `verify` job **cannot** be merged into `development` or
   `main`.
2. The required-check list contains only checks that run on **every** PR to those
   branches — no path-filtered workflow among them.
3. `codeql.yml` runs `security-extended` and contains no step that cannot
   execute.
4. The CodeQL findings from that suite are triaged to zero before CodeQL itself
   becomes a required check.
5. Renaming a job surfaces as a failing merge rather than as a silently dropped
   requirement.

## User stories

- **As the maintainer**, I want a red PR to be unmergeable, so that "I watched it
  go green" stops being the enforcement mechanism.
- **As the maintainer**, I want CodeQL to run the deeper suite, so that a green
  result is evidence rather than a default.
- **As a contributor**, I want a failing check to block my own PR, so I find out
  from the machine rather than from review.

## Requirements

### Must have — phase 1: gate what already passes

1. **Add `required_status_checks` to the ruleset** covering `main` and
   `development`, listing exactly the checks that run on every PR to them:

   | Check | Workflow | Runs on every PR? |
   |---|---|---|
   | `verify` | `ci.yml` | yes — `pull_request:` unfiltered |
   | `goreleaser-check` | `ci.yml` | yes |
   | `Analyze (go)` | `codeql.yml` | yes — `pull_request: [main, development]` |
   | `Analyze (actions)` | `codeql.yml` | yes |

2. **`reproducible-build` must NOT be required.** It is path-filtered to
   `.goreleaser.yaml`, `Makefile`, `go.mod` and its own file. A required check
   that does not run never reports, and the PR waits for it forever. This is the
   single most likely way to make the repository unmergeable, so it is stated as
   a requirement rather than left to be remembered.

3. **`CodeQL` is not required yet.** That is the results check posted by code
   scanning, and it is phase 3 — after the deeper suite has been triaged.

4. **Note the bypass posture, and decide it once.** The ruleset configures no
   bypass actor. Once checks are required, merging a red PR means disabling the
   ruleset — which drops `deletion` and `non_fast_forward` on both branches with
   it, until someone remembers to turn it back on.

   The case that makes this real is not "I want to merge broken code": it is CI
   being red for reasons unrelated to the change, such as a runner image bumping
   its Go toolchain. `reproducible-build.yml` exists because that happens.

   Either add `bypass_actors` for the admin role or record that its absence is
   intentional. **Not a prerequisite for phase 1** — the ruleset can be edited on
   any calm day, and adding a bypass later is neither harder nor more
   constrained. For a solo repository, no bypass is defensible: the switch is
   fine when you are the only person holding it.

5. **`strict` (require branches up to date) is off.** With one maintainer and a
   squash-merge workflow it converts every merge into a rebase, for a race that
   this repository's test suite would catch anyway.

### Must have — phase 2: deepen the analysis

6. **`queries: security-extended`.** Per GitHub's documentation:
   `security-extended` is "queries from the default suite, plus lower severity
   and precision queries". `security-and-quality` adds "maintainability and
   reliability queries" on top — rejected here because `make check` already runs
   `gofmt` and `go vet`, and duplicating style findings into the security surface
   is how a security tab becomes unread.

7. **Delete the dead scaffolding**: the `Run manual build steps` step, and the
   template's instructional comment blocks. Keep the comments that state a
   decision this repository made; delete the ones GitHub ships to everybody.

8. **Fix `actions/setup-go@v4` → `@v5`.** It is v5 in `ci.yml` and v4 here — the
   drift `#122`'s Dependabot `github-actions` entry would surface anyway, fixed
   in passing because it is in the file being edited.

9. **Land non-blocking, then triage to zero.** Whatever `security-extended`
   reports is reviewed and either fixed or dismissed with a reason.

### Must have — phase 3

10. **Add `CodeQL` to the required checks** once phase 2's findings are at zero.

### Nice to have

- A `codeql-config.yml` for future query customisation. Not needed yet: with one
  suite and no exclusions there is nothing for it to hold.
- Recording in `CLAUDE.md` that the required-check list exists, so a job rename
  is known to need a ruleset update.

### Out of scope

- **Path filters to exclude `testdata/` and `_test.go`.** *Investigated and
  dropped:* GitHub's documentation limits `paths`/`paths-ignore` to interpreted
  languages, or compiled languages analysed **without** building — "currently
  supported for C/C++, C#, Java and Rust". Go is absent, and this setup uses
  `autobuild`. Fixture noise, if it appears, is handled by triage or
  `query-filters`, not by path exclusion. This was offered during spec Q&A
  before being checked; it cannot be built as described.
- **Scanning `scripts/*.sh`.** CodeQL has no shell support. A shell linter is a
  different tool and a different decision.
- **Third-party SAST, secret scanning, dependency review.** Each is its own
  spec.
- **Changing the default branch to `development`.** Related (it would also fix
  Dependabot's security-update carve-out from `#122`) but a separate repository
  decision.

## Data model

None. This is CI configuration and a repository ruleset.

## API changes

None. One workflow file, and a ruleset edited through the GitHub API or UI:

```
PUT /repos/ymedlop/kuberoutectl/rulesets/{id}
  rules[].type = "required_status_checks"
  parameters.required_status_checks[].context = "verify" | "goreleaser-check"
                                              | "Analyze (go)" | "Analyze (actions)"
  parameters.strict_required_status_checks_policy = false
```

## UI changes

None in the CLI. In GitHub: PRs gain a merge block until the four checks pass.

## Edge cases

1. **A required check that never runs.** `reproducible-build` is path-filtered;
   requiring it leaves every unrelated PR waiting on a check that will not
   report. Requirement 2.
2. **A renamed job silently stops being required.** The ruleset matches by check
   name. Renaming `verify` in `ci.yml` leaves the ruleset requiring a name
   nothing produces — and GitHub does not warn. The rename would then block every
   PR (waiting for the old name), which is loud, but the reverse — adding a job
   and forgetting to require it — is silent.
3. **`security-extended` reports a backlog on first run.** Handled by sequencing:
   the suite lands non-blocking, findings are triaged, and only then does CodeQL
   gate. This is why phase 3 exists rather than being folded into phase 2.
4. **A PR from a fork.** Whether code scanning reports on fork PRs in a public
   repository, and with what token, is **not verified here** — it must be checked
   before CodeQL becomes required, because a check that cannot run on a fork PR
   would make outside contributions unmergeable. See Testing criteria.
5. **The maintainer needs to merge something red.** *Corrected during Gate 3.5:*
   an earlier draft of this spec said an admin could bypass the ruleset, and
   reasoned that a gate nobody can lift in an emergency is a gate that gets
   deleted in an emergency. The reasoning stands; the fact was wrong.

   The ruleset has `bypass_actors: []` — **no bypass is configured for anyone**,
   at any role. Once required checks are added, a red PR cannot be merged by
   anybody, and the only escape is editing or disabling the ruleset. Which is
   precisely the outcome the reasoning was trying to avoid.

   **This needs a decision before phase 1 lands** (see Requirements). Adding
   `bypass_actors` for the repository admin with `bypass_mode: pull_request`
   keeps the gate meaningful for the normal path while leaving a lever that does
   not require dismantling it. Not doing so is also defensible for a solo
   project — but it should be chosen, not inherited from a default nobody read.
6. **A dismissed finding recurs.** Dismissals in code scanning are per-alert; the
   same pattern reintroduced elsewhere is a new alert. Expected, not a defect.
7. **CodeQL's scheduled run finds something between PRs.** It lands in the
   Security tab and blocks nothing, since the required check is evaluated per PR.
   Accepted: the weekly run is a safety net, not a gate.

## Testing criteria

**Happy path**

- A PR with a deliberately failing test shows `verify` red and the merge button
  blocked.
- A PR touching only `README.md` merges without waiting on
  `reproducible-build`.
- `codeql.yml` runs and reports under `security-extended`, with the workflow
  containing no unreachable step.

**Edge cases**

- Requirement 2 asserted directly: open a docs-only PR after the ruleset change
  and confirm it is mergeable rather than pending.
- Confirm the four required contexts match the check names GitHub actually
  reports — `gh pr checks` on a live PR, not the workflow's `name:` fields, which
  differ for matrix jobs (`Analyze (go)`, not `analyze`).
- **Fork behaviour must be established before phase 3**, per edge case 4. If a
  fork PR cannot produce the `CodeQL` check, requiring it is rejected and the
  reason recorded here.

**Explicitly not covered**

- Nothing tests the ruleset itself; it is repository state, not code. The
  verification is manual and one-off, and this spec's phase 1 is not "done" until
  a red PR has been observed to be unmergeable.

## Dependencies

- Admin access to the repository ruleset.
- The check names as GitHub reports them, which requires one live PR to read.
- No changes to the CLI, its tests, or the verification ladder.

---

## Gate 1

**Completeness**
- [x] Problem stated concretely — the default suite, the dead step, and the empty
      `required_status_checks` list, each verified against the repository rather
      than assumed.
- [x] Goal measurable — five outcomes, two of them negative (no path-filtered
      check required; no silent loss of a requirement).
- [x] Three user stories.
- [x] Requirements split must / nice / out of scope, and phased.
- [x] Out of scope names what was investigated and dropped, with the reason.

**Data model / API**
- [x] Not applicable, stated rather than omitted.
- [x] The ruleset change written out as the API call it is.

**Quality**
- [x] Seven edge cases, including the one most likely to make the repo
      unmergeable.
- [x] Testing criteria for happy path and edge cases, including one that must be
      established *before* phase 3 rather than after.
- [x] Dependencies listed.
- [x] An unverified area named as unverified (fork PR behaviour) rather than
      assumed either way.

**Corrected during Q&A**: path filters for `testdata/` and `_test.go` were
offered as an in-scope item and then checked against GitHub's documentation,
which excludes Go. Moved to out of scope with the quotation, rather than left as
a requirement nobody could satisfy.

**Gate 1: PASSED**
