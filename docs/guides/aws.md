# AWS (EKS) guide

How to use `kuberoutectl` to discover, inspect, and route to **EKS** clusters
across multiple accounts and profiles, and to keep those profiles healthy —
including the corporate **IAM Identity Center / Entra (myapplications.microsoft.com)**
sign-in flow. See the [shared model](README.md) for the concepts and the
credential-health spectrum referenced below.

## Prerequisites

- The **AWS CLI v2** (`aws`) installed and on your `PATH`. `kuberoutectl`
  resolves it (config path → managed runtime → `PATH` → error) and never bundles
  it.
- One or more profiles configured in `~/.aws/config` / `~/.aws/credentials`.
- `kubectl` (only needed once you `target use` a cluster).

```console
$ kuberoutectl doctor
CHECK        STATUS  DETAIL
aws (aws)    ok      /usr/bin/aws
```

## AWS auth models (why credentials differ from Azure)

Unlike Azure's single login, AWS access is **per profile**, and profiles
authenticate in different ways. `kuberoutectl` classifies each profile so the
health and the suggested action make sense:

| Auth type | How it's recognized              | Health when working | Renew path                     |
|-----------|----------------------------------|---------------------|--------------------------------|
| `sso`     | profile has `sso_start_url`      | `valid`             | `aws sso login`                |
| `role`    | assumes a role / source profile  | `valid`             | `aws sso login`                |
| `static`  | long-lived access keys           | `static`            | **none** — rotate keys manually|
| `unknown` | can't be determined              | `unknown`/`error`   | manual                         |

The key idea: **static keys have no expiry to renew**, so their action is
`none`, not `renew`. `kuberoutectl` will refuse to "renew" them and instead tell
you to update `~/.aws/credentials`.

## 1. Sign in

For SSO / Identity Center profiles:

```bash
aws sso login --profile <profile>
# or, if your profiles share an [sso-session]:
aws sso login --sso-session <session-name>
```

For static-key profiles there is nothing to sign into — the keys are already in
`~/.aws/credentials`.

## 2. Discover clusters across every profile

```console
$ kuberoutectl sync aws
Syncing aws ...
  → listing profiles
  → profile 1/3: default
  → profile 2/3: prod-sso
  → profile 3/3: legacy-static
Synced provider: aws
  sources:     3
  credentials: 3
  scopes:      2
  targets:     2
```

`sync aws` enumerates `aws configure list-profiles`, then per profile runs
`aws sts get-caller-identity`, reads the profile's region, and calls
`aws eks list-clusters` + `aws eks describe-cluster`. Each **account** becomes a
**Scope**; each EKS cluster becomes a **Target**.

> **A profile can't authenticate?** That profile's credential is marked
> `expired`/`renew` (SSO) and contributes no clusters, but the sync as a whole
> still succeeds for the profiles that work.

```console
$ kuberoutectl target list --provider aws
ALIAS               PLATFORM  REGION        HEALTH  PROVIDER
eks-prod-frankfurt  eks       eu-central-1  valid   aws
```

The **ALIAS** is a short, stable handle you can pass to `target use`,
`target inspect`, and `target label` instead of the full cluster ARN. Add
`--wide` (or `-o json`) to see the ARN; filter with `--provider aws` or a
selector such as `-l env=prod` or `-l "region in [eu-central-1, eu-west-1]"`.

> **Region note:** discovery scans each profile's **default region** only.
> If a profile has clusters in multiple regions, add a per-region profile (or
> set the region you care about) until multi-region scanning lands — it's on the
> roadmap in `TODO.md`.

## 3. Check credential health

Health is a spectrum, and AWS is where it shows its value — a static key and an
expired SSO session look very different:

```console
$ kuberoutectl credential list
ID                 PROVIDER  IDENTITY                                                              HEALTH   ACTION
aws:default        aws                                                                             expired  renew
aws:legacy-static  aws       arn:aws:iam::222222222222:user/ci-bot                                 static   none
aws:prod-sso       aws       arn:aws:sts::111111111111:assumed-role/AWSReservedSSO_Platform/yeray  valid    use
```

- `aws:prod-sso` — SSO session valid → `use`.
- `aws:default` — SSO session expired → `renew`.
- `aws:legacy-static` — long-lived keys → `static` / `none` (nothing to renew).

## 4. Renew when a session expired

```console
$ kuberoutectl credential renew aws:default
Renewed credential: aws:default
Run `kuberoutectl sync` to refresh health.
```

For `sso`/`role` profiles this runs `aws sso login --profile <profile>` (browser
flow). For **static** profiles it refuses with a clear message rather than
pretending:

```
profile "legacy-static" uses non-renewable credentials;
update ~/.aws/credentials or re-run `aws configure`
```

Re-run `kuberoutectl sync aws` afterward to refresh cached health.

## 5. Route kubectl at a cluster

```console
$ kuberoutectl target use eks-prod-frankfurt   # the alias — or the full ARN
Fetching credentials into ~/.kube/config ...
Now using target: eks-prod-frankfurt (eks-prod-frankfurt)
kubeconfig updated and set as the current context.
```

This runs `aws eks update-kubeconfig --name <cluster> --region <region>
--profile <profile>` using the profile recorded during discovery, merging the
cluster into `~/.kube/config` and setting it current. Use `--no-kubeconfig` to
record the selection without touching your kubeconfig.

```bash
kubectl config current-context
kubectl get nodes
```

## 6. Corporate SSO: discover every account you can reach (Entra / IAM Identity Center)

If your company federates AWS through **myapplications.microsoft.com** (Microsoft
Entra) into **IAM Identity Center**, you may have access to many accounts and
roles but only a few profiles configured locally. You can enumerate them all
from the CLI rather than hand-writing `~/.aws/config`.

Manual approach (works today with just the AWS CLI):

```bash
aws sso login --sso-session <session-name>
aws sso list-accounts --access-token <token>          # accounts you can reach
aws sso list-account-roles --account-id <id> ...       # roles per account
```

> **`kuberoutectl aws sso populate`** — a helper that automates exactly this:
> after `aws sso login`, it reads your `[sso-session]`, calls
> `aws sso list-accounts` / `list-account-roles`, and appends one
> `kr-<account>-<role>` profile per account into `~/.aws/config` (idempotently —
> it never rewrites profiles you already have). One preferred role per account
> (defaults to `AdministratorAccess`, override with `--role`), with optional
> `--region`.
>
> ```bash
> kuberoutectl aws sso populate --sso-session <session-name>
> kuberoutectl sync aws          # now discovers clusters in every populated account
> ```

If there's no valid SSO token, the command tells you to sign in first:

```
not signed in to SSO — run `aws sso login --sso-session <session-name>`
```

## 7. Organize across accounts

Same model as Azure — user labels survive resyncs, collections are live views:

```bash
kuberoutectl target label add eks-prod-frankfurt env=prod
kuberoutectl collection create prod --selector env=prod
kuberoutectl collection show prod
```

Because collections are label-driven, a single `env=prod` collection can hold
both AKS and EKS clusters.

## Capability summary (AWS)

| Capability          | AWS  | Notes                                                    |
|---------------------|------|----------------------------------------------------------|
| Discover scopes     | yes  | accounts via `sts get-caller-identity` per profile       |
| Credential renew    | yes* | `aws sso login` for sso/role; **static keys not renewable** |
| Switch context      | yes  | `aws eks update-kubeconfig`                               |
| Static credentials  | yes  | long-lived keys reported as `static`/`none`              |

## Troubleshooting

- **`not signed in to SSO`** — run `aws sso login --sso-session <name>` (or
  `--profile`), then retry.
- **A profile shows `expired`/`renew` after sign-in** — the token cache may be
  for a different `sso_start_url`; confirm the profile's `sso_start_url` matches
  the session you logged into.
- **Clusters missing from an account** — discovery only scans the profile's
  default region (see the region note above).
- **`renew` refused on a profile** — it uses static keys; rotate them in
  `~/.aws/credentials` or via `aws configure`. This is expected, not a bug.
- **`aws` not found** — install AWS CLI v2 or set an explicit path in config;
  `kuberoutectl doctor` shows what it resolved.
