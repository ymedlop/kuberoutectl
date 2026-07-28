---
title: AWS (EKS)
description: "Manage EKS clusters across AWS accounts and profiles with kuberoutectl — including IAM Identity Center (SSO) sign-in, credential health, and kubectl routing."
parent: Provider guides
nav_order: 2
---

# AWS (EKS) guide

How to use `kuberoutectl` to discover, inspect, and route to **EKS** clusters
across multiple accounts and profiles, and to keep those profiles healthy —
including the corporate **IAM Identity Center / Entra (myapplications.microsoft.com)**
sign-in flow. See the [shared model](index.md) for the concepts and the
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
> still succeeds for the profiles that work. `sync` prints a diagnostic naming
> the profile and the fix (e.g. `run 'aws sso login --profile default'`), so an
> expired token is never silent. Add `--verbose` to also see the raw `aws …`
> command that failed and its stderr.

```console
$ kuberoutectl target list --provider aws
ALIAS               PLATFORM  VERSION  REGION        HEALTH  PROVIDER
eks-prod-frankfurt  eks       1.29     eu-central-1  valid   aws
```

The **ALIAS** is a short, stable handle you can pass to `target use`,
`target inspect`, and `target label` instead of the full cluster ARN. Add
`--wide` (or `-o json`) to see the ARN; filter with `--provider aws` or a
selector such as `-l env=prod` or `-l "region in [eu-central-1, eu-west-1]"`.
`target inspect` also reports the cluster's **Kubernetes server version**.

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

### When several profiles reach the same cluster

An EKS cluster ARN identifies the **account**, not the profile. So if two
profiles authenticate into the same account and both can describe a cluster, they
are two ways into one cluster — not two clusters. `kuberoutectl` lists it once
and records every profile that reaches it:

```console
$ kuberoutectl target list --provider aws
ALIAS               PLATFORM  VERSION  REGION        HEALTH  PROVIDER  PROFILES
eks-prod-frankfurt  eks       1.29     eu-central-1  valid   aws       ops,prod-sso
eks-prod-ireland    eks       1.30     eu-central-1  valid   aws       prod-sso
```

The `PROFILES` column appears only when some cluster has a choice. The
operability verdict is **not** a column — the table is for scanning a fleet, and
the verdict is better asked as a question:

```console
$ kuberoutectl target list --provider aws -l operable=true      # I am admitted to these
$ kuberoutectl target list --provider aws -l operable=unknown   # nothing could be told
$ kuberoutectl target list --provider aws -l operable=false     # confirmed refusals
```

`operable` sits alongside `region`, `platform` and `health` as a selector key, so
it composes with the rest of the grammar and with collections. Its three values
are `true`, `false` and `unknown`, and `unknown` is always queryable rather than
absent — in most fleets it is the largest of the three, and the one worth
enumerating.

Per profile, the verdict lives in `target inspect`, which breaks down each
profile's health *and* whether the cluster admits it — two different questions,
answered separately:

```console
$ kuberoutectl target inspect eks-prod-frankfurt
...
Access check  api (a profile absent from the list will be refused)
profile  ops       valid    use    operable      (primary)
profile  prod-sso  expired  renew  not operable
```

Pick one with `--profile`. The choice is remembered, so a later bare
`target use` reuses it:

```console
$ kuberoutectl target use eks-prod-frankfurt --profile prod-sso
Now using target: eks-prod-frankfurt (eks-prod-frankfurt) via prod-sso
kubeconfig updated and set as the current context.

$ kuberoutectl current
Target   eks-prod-frankfurt (eks-prod-frankfurt)
Provider aws
Profile  prod-sso
```

A profile that cannot reach the cluster is rejected before anything runs, naming
the ones that would work.

**Which clusters a profile reaches is discovered, not assumed.** `eks:ListClusters`
cannot be scoped below account/region, but `eks:DescribeCluster` is evaluated per
cluster — so a profile that lists everything may still be denied on individual
clusters. Each denial is reported during sync, which turns an undocumented access
map into ordinary output:

```console
$ kuberoutectl sync aws
  → profile "ops" cannot describe cluster "eks-prod-ireland" in eu-central-1 — skipping it for this profile
```

### Authenticating is not the same as being admitted

Everything above is **IAM** reachability. Operating *inside* a cluster
additionally requires an EKS **access entry** — a Kubernetes-side authorization
layer. A profile can describe a cluster, activate cleanly, and still get
`Forbidden` from `kubectl`.

`sync aws` reads that layer too — `aws eks list-access-entries`, **one call per
cluster**, for every cluster whose authentication mode permits a conclusion — and
prefers a profile the cluster actually admits, **even over a healthier one**.
Renewing an expired session is one `aws sso login`; a missing access entry cannot
be fixed from this CLI at all.

A cluster reached by a single profile is checked too. There is nothing to
*choose* there, but there is still something to *know*: whether that one way in
will be refused. `sync` reports how many clusters it listed entries for, so the
cost is visible:

```console
$ kuberoutectl sync aws
  → discovered 12 cluster(s); listed access entries for 9
```

The three that cost nothing are `CONFIG_MAP` clusters, where the mode already
came back with `describe-cluster` and access entries do not apply.

What can be concluded depends on the cluster's `authenticationMode`, and only a
*negative* answer depends on it:

| `authenticationMode` | Profile listed | Profile absent |
|---|---|---|
| `API`                | operable       | **not operable** |
| `API_AND_CONFIG_MAP` | operable       | **unknown** — `aws-auth` may still grant it |
| `CONFIG_MAP`         | *(no entries exist)* | **unknown** — access entries do not apply |

**`unknown` is the normal answer, not a failure.** Clusters created through the
API, the SDKs or CloudFormation default to `CONFIG_MAP`, and reading `aws-auth`
would require working `kubectl` access to the very cluster being asked about.
So kuberoutectl reports what it can establish and says nothing where it cannot:

```console
$ kuberoutectl target use eks-prod-frankfurt --profile prod-sso
Warning: prod-sso has no access entry on this cluster at the last sync; kubectl
         may return Forbidden. ops did have one.
Now using target: eks-prod-frankfurt (eks-prod-frankfurt) via prod-sso
```

It warns rather than refuses — the verdict is from the last sync and may be
stale, and going into a cluster to diagnose exactly this is legitimate. A profile
whose verdict is `unknown` produces no warning at all.

### Asking again, without a full resync

`target use` and `target inspect` take **`--refresh`**, which re-establishes
operability against the cluster instead of trusting the last sync — one API call,
for the cluster you named. The case it exists for is narrow and common: *you have
just been granted access and want to know whether it landed.*

```console
$ kuberoutectl target use eks-prod-frankfurt --refresh
ops holds an access entry on this cluster.
Now using target: eks-prod-frankfurt (eks-prod-frankfurt)

$ kuberoutectl target inspect eks-prod-frankfurt --refresh
```

Under `--refresh` the answer is reported in **both** directions — an admission is
as much of an answer as a refusal — and the "at the last sync" wording drops,
because it no longer applies.

Without the flag nothing is called: `sync` already covers every cluster, so a
live check buys freshness rather than coverage. Nothing else re-checks either —
`target list` renders a fleet, and one call per row on every display is the cost
that is never worth paying. The MCP `use_target` and `get_target` tools take the
same `refresh` argument with the same default, so an agent and a human are never
told different things about the same cluster.

> **Still not covered.** An access entry answers "are you admitted", not "may
> you do X" — a restrictive access policy still yields `Forbidden` on specific
> verbs. `kubectl auth can-i` remains the way to check that. Checking also needs
> `eks:ListAccessEntries`; without it the verdict is `unavailable` and every
> profile reads `unknown`, which `sync` names.

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

> **`kuberoutectl setup aws-sso`** — a helper that automates exactly this:
> after `aws sso login`, it reads your `[sso-session]`, calls
> `aws sso list-accounts` / `list-account-roles`, and appends one
> `kr-<account>-<role>` profile per account into `~/.aws/config` (idempotently —
> it never rewrites profiles you already have). One preferred role per account
> (defaults to `AdministratorAccess`, override with `--role`), with optional
> `--region`.
>
> ```bash
> kuberoutectl setup aws-sso --sso-session <session-name>
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
- **A `sync` returns fewer targets than expected** — re-run with `--verbose` to
  see every `aws` command `kuberoutectl` issues, its exit code, and the CLI's
  own stderr (an expired-token profile shows its failure inline).
