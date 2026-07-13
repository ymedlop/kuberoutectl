# Targets

Targets are selectable Kubernetes destinations.

## Rules

- Keep `Target` distinct from `Scope`.
- Targets may represent managed clusters now and kubeconfig contexts later.
- Targets can have system labels and user labels.
- System labels may change during sync.
- User labels must survive sync.
