# Spec: Verbose provider CLI tracing + AWS expired-token visibility

**Created**: 2026-07-24
**Status**: draft
**Author**: Yeray Medina López
**Epic**: none

---

## Problem

When a provider's cloud CLI fails during `sync` — most visibly an expired AWS
SSO/role token — `kuberoutectl` gives the operator no usable signal. Today
`sync aws` with an expired token prints only counters (`credentials: N,
targets: 0`) while the underlying `aws sts get-caller-identity` failure and its
stderr (`Error loading SSO Token: Token has expired`) are silently discarded.
The credential *is* correctly recorded as `HealthExpired`, but nothing on the
sync path tells the operator *why* discovery came back empty, and there is no
way to observe how kuberoutectl drives the cloud CLIs under the hood.

## Goal

Two visibility improvements delivered together:

1. **Default-on auth-failure diagnostic**: `sync aws` surfaces a human-readable
   progress line when a profile's STS identity check fails, naming the profile
   and the likely cause + remedy — without any flag.
2. **Opt-in `--verbose` CLI tracing**: a global `--verbose`/`-v` flag makes
   kuberoutectl emit, for every external cloud-CLI invocation, the command
   (name + args), its exit code, and its stderr **when the command fails**.
   Provider-agnostic — works for azure, aws, gcp, kubeconfig alike.

Success = running `sync aws` with an expired token clearly shows the token is
expired by default, and `--verbose` reveals the exact `aws …` commands being
run and the CLI's own error output on failure.

---

## User Stories

- **As an operator**, when I run `kuberoutectl sync aws` and a profile's token
  has expired, I see a line telling me which profile failed and that I likely
  need to run `aws sso login`, so I'm not left guessing why targets came back
  empty.
- **As an operator debugging a provider**, I run `kuberoutectl sync aws
  --verbose` and see each `aws` command kuberoutectl invokes, its exit status,
  and the CLI's stderr on failure, so I can tell whether the problem is my
  environment, my credentials, or kuberoutectl.

---

## Requirements

### Must-have
- Global persistent flag `--verbose` / `-v` (boolean, default false) on the
  root command, mirroring how `--output` is wired (declared on root,
  resolved in `PersistentPreRunE`).
- Verbose tracing is provider-agnostic: implemented as a `CommandRunner`
  wrapper around `ExecRunner` at the `execx` layer, so all four providers get
  it for free without provider-specific conditionals.
- When verbose is enabled, each external command emits to **stderr**:
  - the command line (`name` + args),
  - its exit code / success,
  - the command's **stderr** when the command failed (non-zero/exec error).
- Verbose output goes to **stderr** so it never pollutes `--output json` on
  stdout.
- Default-on (no flag) AWS diagnostic: in `aws.Provider.Discover`, when the STS
  identity check for a profile fails, emit a `prog.Step` naming the profile and
  the likely cause + remedy (e.g. `profile "dev": identity check failed —
  token likely expired; run 'aws sso login --profile dev'`).
- Existing resilient discovery behavior is preserved: a failed STS check still
  yields a credential with `HealthExpired`/`ActionRenew` and discovery
  continues; the diagnostic is additive, never fatal.

### Nice-to-have
- The AWS diagnostic message distinguishes SSO/role (renewable → suggest login)
  from static-key failures (suggest checking credentials), reusing the existing
  `classifyAuth` result already computed in `Discover`.

### Security: secret redaction (in scope)

The trace prints command arguments, and at least one command carries a secret
as an argument — `aws sso list-accounts --access-token <token>` (and
`list-account-roles`) in `internal/providers/aws/sso.go`. The trace decorator
therefore **masks the value of any secret-bearing flag** (name contains
`token` / `secret` / `password`) as `***`, in both `--flag value` and
`--flag=value` forms. stdout is never traced (Azure/GCP access tokens and the
`kubectl config view --raw` dump live there), and no known CLI echoes these
tokens on stderr — so masked args are the only defense needed.

### Out of scope
- Replicating the explicit auth-failure `prog.Step` diagnostic in azure, gcp,
  and kubeconfig providers. `--verbose` already covers their raw CLI traces;
  their per-provider human diagnostics are a follow-up by the same convention.
- Verbose dumping of command **stdout** (only command + exit + stderr-on-failure
  in this change).
- Log levels, `--quiet`, structured/JSON logging, or writing traces to a file.
- Redaction of secrets appearing in a command's **stderr** (free-form text; no
  known provider CLI echoes credential material to stderr — args are the only
  concrete leak, and those are masked).
- Changing the sync summary schema (`syncSummary`) or adding a health breakdown
  to the summary output.

---

## Data Model

No persistence or domain-type changes. `HealthExpired` / `ActionRenew` already
exist and are already assigned. No cache schema change.

---

## API / CLI Changes

New global flag, applies to every subcommand:

```
--verbose, -v    print external CLI commands, exit codes, and stderr on failure (to stderr)
```

Example — expired token, default output:

```
$ kuberoutectl sync aws
Syncing aws ...
  → listing AWS profiles (aws configure list-profiles)
  → found 1 profile(s)
  → validating identity for profile "dev" (1/1)
  → profile "dev": identity check failed — token likely expired; run 'aws sso login --profile dev'
  → discovered 0 cluster(s)
Synced provider: aws
  sources:     1
  credentials: 1
  scopes:      0
  targets:     0
```

Example — same, with `--verbose` (raw trace added to stderr):

```
$ kuberoutectl sync aws --verbose
Syncing aws ...
  → listing AWS profiles (aws configure list-profiles)
[exec] aws configure list-profiles → ok
  → found 1 profile(s)
  → validating identity for profile "dev" (1/1)
[exec] aws sts get-caller-identity --profile dev --output json → exit 255
       stderr: Error loading SSO Token: Token for dev does not exist
  → profile "dev": identity check failed — token likely expired; run 'aws sso login --profile dev'
  → discovered 0 cluster(s)
...
```

(Exact `[exec]` line format is a plan-level detail; the required content is
command + exit status + stderr-on-failure.)

---

## UI Changes

None beyond the stderr text above. No TUI, no color requirement.

---

## Edge Cases

1. **Non-verbose is unchanged for the happy path** — with `--verbose` off and
   all commands succeeding, output is byte-for-byte what it is today (no stray
   trace lines).
2. **`--output json` + `--verbose`** — traces and the AWS diagnostic go to
   stderr only; stdout stays valid parseable JSON.
3. **Command fails with empty stderr** — trace still shows the command and its
   non-zero exit; no blank `stderr:` line is emitted when stderr is empty.
4. **STS succeeds but returns empty account** — existing `identity.Account == ""`
   path still `continue`s; decide whether it also warrants a diagnostic
   (lean: reuse the same failure diagnostic only when `stsErr != nil`, to avoid
   noise on genuinely valid-but-empty identities).
5. **Multiple profiles, mixed health** — each failing profile gets its own
   diagnostic line naming that profile; healthy profiles produce none.
6. **Static-key profile fails** — diagnostic wording should not tell the user to
   run `aws sso login` (that profile has no SSO); nice-to-have branch handles
   this via `classifyAuth`.
7. **Runner constructed before flags parsed** — the verbose wrapper must read a
   shared toggle that `PersistentPreRunE` flips after parsing, since the runner
   is built once in `newApp()` and injected into providers at registration.

---

## Testing Criteria

### Happy path
- `execx` wrapper: with tracing disabled, output is empty and results pass
  through unchanged (stdout/stderr/err identical to the wrapped runner).
- `execx` wrapper: with tracing enabled and a successful command, a trace line
  with command + success is written to the sink; no `stderr:` block.

### Edge cases
- `execx` wrapper: failing command (non-zero/exec error) writes command + exit
  status + the command's stderr to the sink; empty stderr writes no `stderr:`
  block.
- `aws.Discover` (FakeRunner): a profile whose `sts get-caller-identity`
  returns an error produces a diagnostic `prog.Step` naming that profile, AND
  still appends a credential with `HealthExpired`/`ActionRenew` (assert both —
  the resilient behavior must not regress).
- `aws.Discover` (FakeRunner): all-healthy profiles produce **no** diagnostic
  step.
- CLI: `--verbose` is accepted on the root and on `sync <provider>`; default
  (absent) leaves behavior unchanged. Round-trips through `PersistentPreRunE`.

### Verification ladder (per CLAUDE.md)
- `go test ./...`
- `make check`
- `bash scripts/e2e.sh` (fake az/aws/gcloud/kubectl) — add/extend a case that
  drives an expired-token `aws` fake and asserts the diagnostic appears.

---

## Dependencies

- `internal/execx` (`CommandRunner`, `ExecRunner`, `FakeRunner`) — wrapper lives
  here; `FakeRunner` already returns per-command `Err`, enough to test failures.
- `internal/cli/root.go` — flag declaration + `PersistentPreRunE` wiring +
  runner construction in `newApp()` (the shared verbose toggle is set here).
- `internal/providers/aws/aws.go` + `health.go` — diagnostic in `Discover`;
  `classifyAuth` reused for SSO-vs-static wording.
- `internal/providers.Progress` — the diagnostic uses the existing `prog.Step`
  mechanism; no new interface.

---

## Design notes (for /spartan:plan)

- Verbose wiring: the real runner is built once in `newApp()` before flags are
  parsed. Wrap `ExecRunner` in a `traceRunner` holding a shared, mutable
  trace config (an `enabled` bool + an `io.Writer` sink). `PersistentPreRunE`
  flips `enabled` and points the sink at `cmd.ErrOrStderr()` after parsing —
  same lifecycle pattern already used for `a.output`.
- Keep the core provider-agnostic: no `verbose` field on `aws.Provider`; the
  wrapper is the only place that knows about tracing. The AWS diagnostic uses
  data already in hand (`stsErr`, `classifyAuth`), not a new capability.
- This preserves the discovery error convention: command failure stays
  resilient (still `continue`s), we only add an *optional diagnostic* — exactly
  what the convention permits.
