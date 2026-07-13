# Testing

Prefer table-driven tests unless another style is clearer.

## What to test

- provider registration,
- JSON cache persistence,
- target selection logic,
- label validation and persistence,
- collection selector resolution,
- health state mapping,
- action hint mapping,
- binary resolution precedence,
- Azure parsing and discovery logic,
- AWS parsing and discovery logic.

## Test style

- Test behavior, not implementation details.
- Use deterministic fixtures.
- Avoid flaky timing-based tests.
- Keep mocks minimal and explicit.
- Prefer focused unit tests first.

## Good patterns

- Table-driven cases for selectors, parsing, and mapping logic.
- Golden-style assertions only when output stability matters.
- Integration tests only for high-value paths that cross package boundaries.

## Avoid

- brittle string assertions for unrelated formatting,
- excessive mocking,
- hidden global state,
- tests that require network access.
