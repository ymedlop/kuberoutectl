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
  format regression masquerade as "not logged in".
- Remove only orphans your own change created; mention pre-existing dead code,
  don't delete it unasked.

## 4. Goal-driven execution

**Define success criteria. Loop until verified.**

Turn tasks into verifiable goals — "add X" becomes "write the failing test for
X, then make it pass". The verification ladder here:

```
go test ./...            # unit: domain, services, providers (fixtures, no cloud)
make check               # fmt + vet + test — the pre-commit gate
bash scripts/e2e.sh      # 4-provider operator flow with fake az/aws/gcloud/kubectl
```

All three must pass before a PR. What fixtures cannot prove (real CLI output
shapes, interactive auth) is an accepted caveat — say so in the PR instead of
pretending coverage.

## Repo workflow

- PRs target `development` (branch-protected); `main` is stable.
- Read `README.md` and `ARCHITECTURE.md` before major changes; evolving
  implementation prompts live in `prompts/claude-code/`.
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
