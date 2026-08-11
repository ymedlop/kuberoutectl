# CLAUDE.md

@AGENTS.md

Behavioral guidelines for working on `kuberoutectl`. AGENTS.md owns the domain
model and architecture rules; this file covers *how to work*. Bias toward
caution over speed — for trivial tasks, use judgment.

## 1. Think before coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

- State assumptions explicitly; if multiple interpretations exist, present
  them — don't pick silently.
- For major changes, present the design (domain mapping, interfaces, provider
  fit) and wait for confirmation before implementing.
- If a simpler approach exists, say so. Push back when warranted — agreeing to
  avoid conflict is a failure mode.
- **Cite only what you checked.** If a decision leans on a precedent in this
  repo, an API contract, or a prior PR, verify it before invoking it. An invented
  justification is worse than an unjustified decision: it survives review,
  because nobody audits the reasoning in a comment the way they audit the code
  under it.
- Anything unclear? Stop, name it, ask.

## 2. Simplicity first

**Minimum code that solves the problem. Nothing speculative.**

- No features, flags, or provider capabilities beyond what was asked.
- No abstractions for single-use code; no configurability nobody requested.
- The registry + capability flags already give extension points — don't invent
  new indirection on top of them.
- Ask: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical changes

**Touch only what you must. Match what's already there.**

- Every changed line should trace directly to the request. Don't "improve"
  adjacent code, comments, or formatting.
- New provider work mirrors the existing package template:
  `parse.go` (pure JSON→struct) + `build.go` (struct→domain) + `health.go` +
  `activate.go` / `renew.go`, fixtures under `testdata/`, FakeRunner tests.
- Error convention in provider `Discover`: an external-CLI *command failure*
  is resilient (fall through, optionally `prog.Step` a diagnostic); a *parse
  failure on a successful command* is a wrapped hard error — never let a
  format regression masquerade as "not logged in". **One exception**: a parse
  failure on a *per-item* call inside a loop (a subscription's cluster list, a
  profile's, a project's) is skipped rather than fatal, so one bad item cannot
  sink the whole sync — but it must emit a `prog.Step` naming the item and
  flagging a possible CLI format change. Skipping it silently is the bug the
  rule exists to prevent, just quieter.
- Remove only orphans your own change created; mention pre-existing dead code,
  don't delete it unasked.

## 4. Goal-driven execution

**Define success criteria. Loop until verified.**

Turn tasks into verifiable goals — "add X" becomes "write the failing test for
X, then make it pass". The verification ladder here:

```
go test ./...            # unit: domain, services, providers (fixtures, no cloud)
make check               # fmt-check + vet + test — the pre-commit gate
bash scripts/e2e.sh      # 4-provider operator flow with fake az/aws/gcloud/kubectl
make docs-reference      # regenerate docs/reference; commit it if it changed
make verify-readme       # README + demo commands still exist in the CLI
```

All five must pass before a PR, and the last two are the ones that get skipped.
They generate or check content from the Cobra tree rather than testing
behaviour, so **a change to the command surface passes the first three rungs and
fails CI** — which is exactly how it happened in #110, where adding a flag left
`docs/reference` stale. Treat them as mandatory whenever a command, subcommand,
flag, or help string is touched: `make docs-reference` rewrites files, so run it
and commit the result, don't just read its output.

What fixtures cannot prove (real CLI output shapes, interactive auth) is an
accepted caveat — say so in the PR instead of pretending coverage.

**A guard is not proven until it has failed.** Before trusting a test that
protects an invariant, break the code it guards — invert the branch, stub the
function to a constant, delete the condition — and confirm *that specific test*
fails. Restore, and say in the PR that you did it.

Assertions that pass no matter what the code does, all seen here:

- `reflect.DeepEqual` against a map that shares identity with the value under
  test (a Go struct copy aliases its map fields — clone the expected value first).
- `strings.Contains` on a whole row of tabular output, satisfied by a
  neighbouring column. Index the cell by header name.
- `json.Unmarshal` into a reused non-nil map: it **merges**, so a later subtest
  reads the earlier one's keys.
- A guard repeating verbatim the condition it is meant to protect, making the
  block unreachable.
- Logic moved to another layer without moving or re-creating the test that
  exercised it — no test looks wrong, and nothing covers the new home.
- A test double carrying a field nothing ever sets: scaffolding designed and not
  written, which reads as covered.

None of these are visible by re-reading the test — each one looks like a normal
assertion. Injection is the only check that finds them.

## Repo workflow

- PRs target `development` (branch-protected); `main` is stable.
- **Promoting `development` → `main`:** do NOT open a plain `development → main`
  PR — squash merges leave `main` un-ancestored, so it 3-way-conflicts. Follow
  the branch-off-`main` recipe in `RELEASING.md` (§Promoting development to main).
- Read `README.md` and `ARCHITECTURE.md` before major changes; evolving
  implementation prompts live in `prompts/claude-code/`.
- **`.planning/` travels with the code.** A feature's spec and plan belong in
  the same PR as the implementation, so they cannot land late or not at all.
  Split onto their own branch once, the spec never got pushed while the feature
  shipped without it. A standalone docs PR is only for work with no
  implementation to ride along with — a ruleset change, a process decision.
- **A shipped spec says so.** On merge, set `**Status**: shipped in #NNN`, and
  amend the body *in place* rather than stacking a correction onto text it now
  contradicts. Until this rule, all seven merged specs still read `draft` while
  describing shipped behaviour. The usual reader of `.planning/` is the next
  agent session, and `draft` reads as live intent — near enough to current to
  be cited as precedent, which is a fabrication with a citation attached.
- Report honestly: failing tests are reported with output, skipped steps are
  named, sandbox limitations are stated as caveats, not hidden.

## Skills (load on demand, not by default)

Load the matching skill *and its `references/`* when a task opens net-new
territory; skip for incremental edits inside patterns already established:

- `go-development` → new packages, CLI wiring, test strategy, errors.
- `cloud-adapters` → new provider integrations, external CLI execution,
  binary resolution.
- `kubernetes-inventory` → targets, labels, collections, selectors,
  persistence semantics.

---

**These guidelines are working if:** diffs stay minimal, design questions come
before implementation rather than after mistakes, provider packages stay
interchangeable in shape, and every PR states what is verified and what is not.
