# Interfaces

Provider adapters should stay behind explicit interfaces.

## Rules

- Core services should depend on interfaces, not concrete CLI details.
- Provider packages should expose capabilities explicitly.
- Discovery, health checks, and renewal should be separated where possible.
- Keep adapter methods small and single-purpose.
