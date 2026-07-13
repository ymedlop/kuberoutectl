# Error handling

## Principles

- Wrap errors with enough context to identify what failed.
- Include the operation, provider, target, or file path when relevant.
- Keep errors useful for operators and for debugging.

## Good error shape

- what the code was trying to do,
- which provider or target was involved,
- which external command or file caused the problem,
- whether the failure is retryable or requires user action.

## Examples of useful context

- `discover azure subscriptions: az account list failed`
- `load cache: invalid JSON in targets.json`
- `select target: unknown label key env`
- `renew credential: AWS profile has no refresh flow`

## Avoid

- swallowing provider-specific details,
- returning bare low-context errors,
- overusing panic,
- turning every error into a generic message.
