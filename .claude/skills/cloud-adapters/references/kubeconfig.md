# kubeconfig

Use the kubectl CLI to read the merged kubeconfig; never parse the file directly.

## Shape

A static artifact, not an authenticator: kubeconfig file → AccessSource;
`clusters[]` → Scopes (kind `cluster`); `users[]` → Credentials; `contexts[]` →
Targets (kind `context`). See `internal/providers/kubeconfig/`.

## Commands used

- `kubectl config view --raw -o json` — the single discovery read (honors
  `$KUBECONFIG` merging exactly as kubectl does)
- `kubectl config use-context <name>` (Activate — the context already exists,
  so activation is a switch, not a credential fetch)

## Notes

- **Nothing is renewable**: `CanRenew=false`, `Renew` returns
  `providers.ErrUnsupported`. Client cert / token / basic auth →
  `static`/`none`; `exec` / auth-provider users → `unknown`/`none` (their
  plugin refreshes them outside our view). Never map to renew.
- Classify auth by the **presence** of material only (`client-certificate-data`,
  `token`, `exec`, ...) — never store the secret values in the cache.
  Order matters: exec and auth-provider win over static material.
- A context may reference a missing `users[]` entry — treat as unknown/none,
  keep the target.
- An empty kubeconfig is not an error; it yields nothing.
- Follow-up in TODO.md: parse client-cert `notAfter` (stdlib `crypto/x509`)
  for real valid/expiring/expired health.
