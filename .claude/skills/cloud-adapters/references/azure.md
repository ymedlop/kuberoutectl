# Azure

Use Azure CLI as the source of truth for Azure account and subscription state.

## Typical operations

- `az login`
- `az account list`
- `az account show`
- `az account set --subscription <id>`

## Notes

- Subscription is the primary scope abstraction.
- Discovery should focus on subscriptions and AKS clusters.
- Credential health should reflect logged-in/account state as far as the CLI can determine.
- Keep parsing deterministic and testable.
