# Persistence

Persist inventory and user organization separately.

## Suggested split

- discovered inventory: providers, sources, credentials, scopes, targets
- user state: labels, collections, selections, preferences

## Rules

- Use JSON for the MVP.
- Do not store cloud secrets in the cache unless absolutely necessary.
- Keep sync deterministic and non-destructive to user labels.
