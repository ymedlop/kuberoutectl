---
name: go-development
description: Use when writing, reviewing, or refactoring Go code in kuberoutectl, especially CLI wiring, services, tests, errors, and package layout.
---

# Go Development

Use this skill when working on Go code in `kuberoutectl`.

## Goals

- Keep code small, idiomatic, and testable.
- Keep Cobra handlers thin.
- Prefer explicit services and interfaces.
- Preserve the provider-agnostic architecture.

## Workflow

1. Read the architecture and domain model.
2. Identify the smallest testable change.
3. Implement the service or provider logic first.
4. Add or update focused tests.
5. Keep CLI handlers as adapters only.
6. Prefer deterministic behavior and clear errors.

## When to use

Use this skill for:
- new Go packages,
- refactors,
- CLI commands,
- JSON cache logic,
- provider drivers,
- selectors and labels,
- tests and build changes.

## References

- `references/package-layout.md`
- `references/testing.md`
- `references/errors.md`
- `references/cli-patterns.md`
