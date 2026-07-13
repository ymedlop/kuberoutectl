# CLI patterns

## Command handlers

Cobra commands should only:
- parse flags and args,
- call services,
- render text or JSON output,
- return errors.

## Do not put in commands

- discovery logic,
- label mutation rules,
- selector evaluation,
- cache persistence,
- provider orchestration.

## Output rules

- Support JSON output when the command exposes inventory or state.
- Keep human-readable output concise and predictable.
- Avoid mixing business logic with formatting.

## Preferred flow

1. CLI receives input.
2. Service computes result.
3. CLI renders result.
4. Errors are wrapped once at the boundary.

## Good examples

- `target list` reads from the target service.
- `collection create` validates selector then stores state.
- `credential renew` delegates to the credential service.
- `sync azure` delegates to provider discovery.
