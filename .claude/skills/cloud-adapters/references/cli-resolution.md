# CLI resolution

Resolve external binaries in this order:

1. explicit path from config
2. managed runtime installed by kuberoutectl
3. PATH lookup
4. clear diagnostic error

## Rules

- Do not hide resolution failures.
- Do not assume the CLI is installed.
- Make resolution behavior testable.
- Keep command execution structured and isolated.
