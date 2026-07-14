---
name: cloud-adapters
description: "Provider adapter patterns for kuberoutectl. Use when implementing or reviewing provider integrations (azure, aws, gcp, kubeconfig, or a new one), external CLI execution, binary resolution, or provider discovery/renewal/activation logic."
allowed_tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
---

# Cloud Adapters

Use this skill when working on provider integrations in `kuberoutectl`.

## Goals

- Keep provider logic isolated behind interfaces; the core stays provider-agnostic.
- Treat external CLIs as adapters, not as the core product.
- Make every provider package interchangeable in shape.

## The package template (mirror it exactly)

Every provider follows the same layout — study `internal/providers/aws/` (the
richest) before writing a new one:

```
internal/providers/<name>/
  <name>.go       # Provider struct, New(), ID(), Capabilities(), Discover()
  parse.go        # pure JSON -> struct; only the fields we consume
  build.go        # struct -> domain entities; ID derivation helpers at top
  health.go       # auth classification + health/action mapping
  activate.go     # ContextActivator (only if CanSwitchContext)
  renew.go        # Renew (or return providers.ErrUnsupported)
  testdata/       # captured CLI JSON fixtures
  *_test.go       # pure funcs vs fixtures; Discover/Activate/Renew via FakeRunner
```

Wiring is exactly two lines in `internal/cli/root.go` (Register + requiredBinary).
If a change needs more than that outside the package, the design is wrong.

## Workflow

1. Decide the provider's shape first: per-login (azure, gcp — one source +
   credential spanning scopes) or per-profile (aws). Document the choice and
   why in the package comment.
2. Map to the domain: what is the Scope? what is the Target? Never collapse them.
3. Write fixtures + failing parse tests, then the pure functions, then Discover.
4. Verify: `go test ./...`, then extend `scripts/e2e.sh` with a fake CLI.

## Gotchas

- **Command failure and parse failure are different events.** A CLI *command
  failure* (exit non-zero) is resilient: fall through to the logged-out/empty
  path, optionally with a `prog.Step` diagnostic. A *parse failure on a
  successful command* is a wrapped hard error
  (`fmt.Errorf("<provider>: parse <cmd>: %w", err)`) — swallowing it makes a
  format regression masquerade as "not logged in". (Caught by review in the
  first GCP implementation.) Exception: per-item failures inside a loop over
  projects/profiles stay silent-and-skipped so one bad item doesn't sink the
  whole sync.
- **Never coerce static credentials into a renew lifecycle.** AWS static keys
  are `static`/`none` (failing ones `error`/`manual`); kubeconfig users never
  map to renew. Capabilities gate the menu; per-credential health picks the
  item — a provider can be CanRenew=true and still hold non-renewable
  credentials.
- **FakeRunner matches the exact argv string.** Keys look like
  `"aws sts get-caller-identity --profile default --output json"`; reordering
  flags in the implementation fails tests with "no response registered", which
  reads as a missing fixture rather than a flag-order change.
- **Target IDs need enough qualifiers to be unique.** GKE cluster names are
  only unique per project+location, so the ID is `gcp:project:location:name`.
  Check the real uniqueness scope before deriving an ID — aliases and user
  labels attach to it forever.
- **Fixtures encode beliefs, not proof.** Real `az`/`aws`/`gcloud` cannot run
  in the sandbox. State the untested surface (flag names, JSON field names,
  interactive auth) as an explicit PR caveat and smoke-test on a real machine.
- **Progress goes to stderr.** `Discover` reports via `prog.Step`; the CLI
  renders it on stderr so `--output json` stdout stays machine-clean. A silent
  slow sync reads as a hang (real report: an expired `az` session blocking on
  interactive auth looked "idle").

## References

- `references/azure.md`
- `references/aws.md`
- `references/gcp.md`
- `references/kubeconfig.md`
- `references/cli-resolution.md`
- `references/interfaces.md`
