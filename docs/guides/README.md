# Provider guides

Hands-on manuals for using `kuberoutectl` with each supported cloud, focused on
the two things operators do most: **managing clusters** (discover, inspect,
route to) and **managing credentials** (check health, renew, re-authenticate).

- [Azure (AKS)](azure.md)
- [AWS (EKS)](aws.md)
- [kubeconfig](kubeconfig.md) — self-hosted / local / handed-to-you contexts

## How the CLI thinks (shared model)

Every provider maps onto the same domain model, so the commands are identical
across clouds — only the underlying CLI differs:

| Concept        | Azure                       | AWS                          | kubeconfig             |
|----------------|-----------------------------|------------------------------|------------------------|
| **Provider**   | `azure`                     | `aws`                        | `kubeconfig`           |
| **AccessSource** | Azure CLI login profile   | each `~/.aws` profile        | the kubeconfig file    |
| **Credential** | login identity (per tenant) | one per profile              | each `users[]` entry   |
| **Scope**      | subscription                | account                      | cluster                |
| **Target**     | AKS cluster                 | EKS cluster                  | context                |
| Underlying CLI | `az`                        | `aws`                        | `kubectl`              |

`kuberoutectl` never stores your secrets. It shells out to the provider CLI you
already use, caches the **inventory** it discovers (names, regions, health,
expiry) under `~/.kuberoutectl/cache/`, and keeps your own organization (labels,
collections, current selection) separately under `~/.kuberoutectl/state/` so a
resync never erases it.

## The universal loop

```bash
kuberoutectl doctor              # 1. is the provider CLI reachable?
kuberoutectl sync <provider>     # 2. discover clusters + credential health
kuberoutectl credential list     # 3. what's valid / expiring / expired?
kuberoutectl target list         # 4. what can I reach?
kuberoutectl target use <id>     # 5. route kubectl at one cluster
```

Everything else — `scope list`, `target inspect`, labels, and collections — is
about slicing that inventory once it's in the cache. Add `-o json` to any
inventory command for scripting.

## Credential health, once

Both guides refer to this spectrum. It is a property of the credential, and it
drives the suggested **action**:

| Health     | Meaning                                   | Action  |
|------------|-------------------------------------------|---------|
| `valid`    | usable now                                | `use`   |
| `expiring` | usable but close to expiry                | `renew` |
| `expired`  | not usable until re-auth                  | `renew` |
| `static`   | long-lived key, no expiry to track        | `none`  |
| `unknown`  | could not be determined                   | `none`  |
| `error`    | the provider CLI failed while checking    | `repair`/`manual` |

`static` is not a failure — it means there is nothing to renew (see the AWS
guide). `kuberoutectl` never coerces a static key into a `renew` action.
