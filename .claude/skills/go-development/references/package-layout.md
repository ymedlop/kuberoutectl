# Package layout

Use a clean separation between wiring, domain, and services.

## Suggested layout

- `cmd/kuberoutectl`: CLI entrypoint and wiring only.
- `internal/domain`: entities, IDs, value objects, selectors, and enums.
- `internal/providers`: provider registry and provider drivers.
- `internal/providers/azure`: Azure-specific discovery and renewal logic.
- `internal/providers/aws`: AWS-specific discovery and renewal logic.
- `internal/providers/kubeconfig`: future kubeconfig support.
- `internal/providers/gcp`: future GCP support.
- `internal/services`: discovery, targets, labels, collections, doctor, cache orchestration.
- `internal/cache`: JSON persistence and state storage.
- `internal/execx`: command execution and binary resolution.
- `internal/config`: configuration loading and defaults.
- `internal/cli`: Cobra commands and output formatting.
- `pkg`: only if public reusable APIs are truly needed.

## Rules

- Keep business logic out of `cmd` and `internal/cli`.
- Keep provider-specific logic inside provider packages.
- Keep shared behavior in services, not command handlers.
- Prefer small packages with clear ownership.
- Avoid creating packages just to create packages.
