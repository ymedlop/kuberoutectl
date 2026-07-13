# Labels

Labels are key/value metadata used for organization and selection.

## Rules

- Validate label keys and values.
- Keep internal labels under a reserved namespace such as `kuberoutectl.io/*`.
- Store user labels separately from system labels.
- Do not let discovery overwrite user labels.

## Examples

- `env=prod`
- `project=payments`
- `team=platform`
