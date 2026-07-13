# CLAUDE.md

@AGENTS.md

## Claude Code guidance for kuberoutectl

Use the repo skills on demand:

- `go-development` → Go code, package layout, tests, errors, CLI wiring.
- `cloud-adapters` → Azure/AWS provider integration, external CLI execution, binary resolution.
- `kubernetes-inventory` → targets, labels, collections, selectors, persistence.

## Workflow

- Read `README.md` and `ARCHITECTURE.md` before major changes.
- Read the latest prompt in `prompts/claude-code/` before implementing.
- Keep `AGENTS.md` as the repo-wide rule source.
- Keep this file short and stable.
