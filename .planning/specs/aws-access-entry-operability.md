# Spec: AWS access-entry operability

**Created**: 2026-07-27
**Status**: shipped in #112, #116, #117, #118, #121
**Author**: Yeray Medina López
**Epic**: none
**Target release**: 1.2.0
**Follows**: `.planning/specs/aws-multi-profile-targets.md` (shipped as #109 + #110)

## Problem

The multi-profile work resolves which profiles can *reach* a cluster by whether
`eks:DescribeCluster` succeeds — an **IAM** permission. Operating inside the
cluster additionally requires an EKS **access entry**, which is a Kubernetes-side
mechanism kuberoutectl does not read. Decision D3 in the previous plan named this
gap and deferred it; the operator has since confirmed their fleet differs at
exactly that layer.

The concrete failure:

```
profile A: describe OK → in CredentialIDs → chosen as default → kubectl: Forbidden
profile B: describe OK → in CredentialIDs →                     kubectl: works
```

Both look identical to the current code, so the default is a coin flip with no
signal that it landed wrong. `target use` prints
`(default — pass --profile to pick another)` precisely because the guess is
unverifiable today. This spec makes it verifiable.

## Goal

`kuberoutectl` distinguishes "this profile can authenticate" from "this profile
has an access entry on this cluster", picks the default accordingly, and is
explicit when it cannot tell.

Verifiable:

1. `sync aws` records, per cluster reachable by more than one profile, which of
   those profiles hold an access entry.
2. `target list` shows an `OPERABLE` column; `target use` with no flag prefers a
   confirmed-operable profile.
3. A cluster whose authentication mode makes the answer unknowable reports
   `unknown` with the reason, never a false negative.
4. No extra API call is made for a cluster only one profile reaches.

## User stories

- As an SRE whose fleet grants EKS access inconsistently across profiles, I want
  `target use <cluster>` to land on a profile that actually works, so I stop
  discovering the wrong choice through a `Forbidden` from `kubectl`.

- As an operator running an ops agent over the inventory, I want the agent to
  pick a cluster it can genuinely operate, so "find a cluster and run this
  analysis" does not fail after it has already switched context.

- As that same operator, I want a cluster whose mode makes this unknowable to say
  so, so I can tell "no access" from "cannot tell" and not act on a guess dressed
  as a fact.

## Requirements

### Must have

**Discovery**

- During `sync aws`, for each cluster in `CredentialIDs` with **more than one**
  entry, call `aws eks list-access-entries --cluster-name <n> --region <r>
  --profile <p>` once per cluster (not per profile — the list covers all
  principals) and match each reaching profile's identity against it.
- Follow `nextToken` pagination. A truncated first page silently drops
  principals, which would read as a confirmed "no".
- Read `accessConfig.authenticationMode` from the existing `describe-cluster`
  response — no additional call. `awsCluster` in `parse.go` does not parse
  `accessConfig` today.

**The three-mode truth table.** Only a *negative* answer depends on the mode:

| `authenticationMode` | Principal listed | Principal absent |
|---|---|---|
| `API` | operable | **not operable** |
| `API_AND_CONFIG_MAP` | operable | **unknown** — `aws-auth` may grant it |
| `CONFIG_MAP` | *(no entries exist)* | **unknown** — entries do not apply |

Clusters created via the API, SDKs or CloudFormation default to `CONFIG_MAP`, so
`unknown` is expected to be common. It must read as a normal answer, not a
failure.

**Identity matching.** An access entry names an IAM principal ARN; STS returns an
assumed-role ARN. They are never equal as strings:

```
sts get-caller-identity  arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_Platform_abc123/yeray
access entry (SSO)       arn:aws:iam::111122223333:role/aws-reserved/sso.amazonaws.com/eu-central-1/AWSReservedSSO_Platform_abc123
access entry (plain)     arn:aws:iam::111122223333:role/PlatformAdmin
access entry (user)      arn:aws:iam::111122223333:user/ci-bot
```

Matching compares **account plus the final role/user name**, discarding the IAM
path and the STS session name. The path is present for SSO roles and absent for
plain ones, so a path-sensitive comparison fails exactly in the SSO case this
feature exists for.

**Primary selection.** Operability outranks health:

```
1. access entry confirmed   → then by health
2. unknown                  → then by health
3. confirmed absent         → then by health
```

Rationale: an expired session is a renewable obstacle (`aws sso login`); a
missing access entry cannot be fixed from this CLI at all. An expired profile
that *would* work is a better default than a healthy one that will be refused,
and the existing `action_hint: renew` already tells the operator what to do.

**Surfaces**

- `target list` gains an `OPERABLE` column, shown only when some target has more
  than one profile — the rule `PROFILES` already follows.
- `target inspect` adds the verdict per profile alongside its health.
- `target use --profile X` where X is **confirmed absent** writes the kubeconfig
  and warns on stderr, naming a profile that did have an entry. It does not
  block: the verdict is cached and may be stale, and entering a cluster to
  diagnose exactly this is legitimate. `unknown` produces no warning.
- MCP `get_target` exposes the same data (it renders `domain.Target`, so new
  fields ship automatically); `use_target` returns the warning as a field rather
  than prose.

**Resilience**, per the provider error convention:

- `list-access-entries` failing (no `eks:ListAccessEntries` permission, throttling,
  an older cluster) is a **command failure** → resilient. The cluster's verdict
  becomes `unknown` with the reason, discovery continues, and `prog.Step` names it.
- A parse failure on a *successful* `list-access-entries` is a wrapped hard error
  — a format change must not masquerade as "nobody has access".

### Nice to have

- `--check-access-entries=false` to skip the extra calls on a slow link.
- Reporting *which* access policy an entry carries (`list-associated-access-policies`),
  distinguishing view-only from admin.

### Out of scope

- **Reading the `aws-auth` ConfigMap.** It requires `kubectl` against each
  cluster, which puts a Kubernetes client on the discovery path and needs working
  access to read — circular for the question being asked. `CONFIG_MAP` clusters
  stay `unknown`.
- **`kubectl auth can-i` probing.** Same objection, plus it needs a kubeconfig
  entry per (cluster, profile) before it can ask anything.
- **Creating or modifying access entries.** kuberoutectl reports; it does not
  grant.
- **Cross-account access entries** (a principal from another account).
- **RBAC beyond the entry.** Holding an access entry with a restrictive policy
  still yields `Forbidden` on specific verbs. This feature answers "are you
  admitted", not "may you do X".
- Extending any of this to Azure or GCP.

## Data model

`domain.Target`, additive:

```go
// OperableCredentialIDs lists the credentials confirmed to hold an access entry
// on this target. A credential in CredentialIDs but absent here is NOT
// necessarily refused — see AccessCheck, which says whether absence is
// meaningful.
OperableCredentialIDs []CredentialID `json:"operable_credential_ids,omitempty"`

// AccessCheck records what the operability check could establish:
// "api" (absence is authoritative), "api_and_config_map" (only presence is),
// "config_map" (nothing), "unavailable" (the call failed), or empty (not
// attempted — a single-credential target).
AccessCheck string `json:"access_check,omitempty"`
```

Both `omitempty`, absent-means-not-checked. No migration, no schema version bump
— the same property that made the previous two fields safe.

A verdict per credential is **derived**, never stored twice:

```
listed in OperableCredentialIDs        → operable
absent, AccessCheck == "api"           → not operable
absent, anything else                  → unknown
```

## API changes

CLI, additive:

```
kuberoutectl target list        # + OPERABLE column
kuberoutectl target inspect     # + per-profile verdict
kuberoutectl sync aws           # + progress steps for the check
```

MCP `use_target` output, additive:

```json
{ "profile": "ops", "profile_source": "default",
  "access_warning": "prod-sso had no access entry at the last sync" }
```

`target list -o json` and `target inspect -o json` keep rendering targets; the new
fields ride along on `domain.Target`. No output shape changes.

## UI changes

```console
$ kuberoutectl sync aws
  → checking access entries for eks-prod-frankfurt (2 profiles)
  → ops holds an access entry; prod-sso does not
  → eks-prod-ireland uses CONFIG_MAP auth — access entries do not apply

$ kuberoutectl target list --provider aws
ALIAS               PLATFORM  REGION        HEALTH   PROVIDER  PROFILES      OPERABLE
eks-prod-frankfurt  eks       eu-central-1  expired  aws       ops,prod-sso  ops
eks-prod-ireland    eks       eu-central-1  valid    aws       prod-sso      unknown

$ kuberoutectl target inspect eks-prod-frankfurt
...
Access check  api (absence is authoritative)
profile  ops       expired  renew  operable      (primary)
profile  prod-sso  valid    use    not operable

$ kuberoutectl target use eks-prod-frankfurt --profile prod-sso
Warning: prod-sso had no access entry at the last sync; kubectl may return
         Forbidden. ops did have one.
Now using target: eks-prod-frankfurt via prod-sso
```

Note the first row: `ops` is primary while `expired`, because renewing is
recoverable and a missing entry is not.

## Edge cases

1. **Single-profile cluster** — no call is made, `AccessCheck` stays empty, and
   the verdict is neither yes nor no. Verifies the cost bound.
2. **`CONFIG_MAP` cluster** — no entries exist; every profile is `unknown`. Must
   never render as "not operable".
3. **`API_AND_CONFIG_MAP`, principal absent** — `unknown`, not a negative. This
   is the asymmetry a naive implementation gets wrong.
4. **SSO role with an IAM path** — the entry carries
   `role/aws-reserved/sso.amazonaws.com/<region>/AWSReservedSSO_X` and STS
   `assumed-role/AWSReservedSSO_X/<session>`. Must match.
5. **Plain role, no path** — `role/PlatformAdmin` vs
   `assumed-role/PlatformAdmin/<session>`. Must match.
6. **IAM user, not a role** — `user/ci-bot` matches a caller identity of
   `user/ci-bot` directly, with no session component.
7. **Two roles with the same name under different paths** — the account+name rule
   matches both. Accepted as a false positive; documented rather than silently
   assumed impossible.
8. **Paginated response** — a cluster with more principals than one page. The
   profile's entry on page 2 must be found, or a real entry reads as absent.
9. **`eks:ListAccessEntries` denied** — resilient: `AccessCheck` becomes
   `unavailable`, everything is `unknown`, discovery continues, and the step
   names the missing permission.
10. **Malformed JSON from a successful call** — hard error, wrapped. Must not
    degrade to "nobody has access".
11. **Operability changes the primary** — the expired-but-operable case above.
    Asserts the ranking is operability-then-health, not the reverse.
12. **All profiles confirmed absent** — the target still names a primary (health
    order), and `target use` warns rather than refusing.
13. **Pre-upgrade snapshot** — no `access_check`, no `operable_credential_ids`.
    Everything is `unknown`; listing and activation behave as before.
14. **A profile that reaches the cluster but whose STS identity is empty** — no
    ARN to match, so `unknown`, never a negative.

## Testing criteria

**Happy path**

- Two profiles, one cluster, mode `API`, only `ops` listed → `ops` operable,
  `prod-sso` not, `ops` primary even when expired.
- `target list` renders `OPERABLE`; `target inspect` renders per-profile verdicts.
- `target use --profile prod-sso` warns on **stderr** and still writes kubeconfig.

**Edge cases**

- One test per numbered case. Cases 4–7 are pure ARN-matching table tests in
  `internal/providers/aws` with no runner — that function is the most
  error-prone piece and must be tested independently of discovery.
- Cases 1, 2, 3, 8, 9, 10, 11, 14 are FakeRunner tests over `Discover`, with
  fixtures per authentication mode.
- Cases 12, 13 are service/CLI level.
- Case 13 reuses the existing hand-written pre-upgrade fixture, extended only if
  it can stay in the pre-upgrade shape.

**Ladder** — `go test ./... -count=1`, `make check`, `bash scripts/e2e.sh`,
**plus `make docs-reference`** if any flag is added (the local ladder does not
cover the generated command reference; a flag change passes all three rungs and
fails CI).

**Stated gap, to repeat in the PR**: no test runs against a real EKS cluster in
any authentication mode. The mode table, the ARN matching and the pagination are
fixture-driven from the documented response shapes. What stays unproven is that
a live `list-access-entries` returns what those fixtures model — in particular
the exact ARN form of SSO entries in this account, which is the single assumption
the whole feature rests on. **Validate that against one real cluster before
merging**, even manually.

## Dependencies

- No new Go modules and no new external binary — `aws` already drives discovery.
- Requires the `eks:ListAccessEntries` IAM permission on each profile being
  checked; its absence is handled, not required.
- Builds on `Target.CredentialIDs` (#109) and the `--profile` selection (#110).
  Independent of `.planning/specs/update-check-notice.md`.

---

## Gate 1

**Completeness**
- [x] Problem clearly stated — carried from D3, now a confirmed environment, with
      the failing sequence written out.
- [x] Goal specific and measurable — four checkable outcomes, including a
      negative one (no call for single-profile clusters).
- [x] At least one user story — three, covering the human and the agent path.
- [x] Requirements split must / nice / out of scope.
- [x] Out of scope exists — six items, each with its reason, including two
      rejected alternatives and their objection.

**Data model**
- [x] Field types correct; both additive and `omitempty`.
- [x] Migration strategy — absent-means-not-checked, no version bump, matching
      the two fields already shipped this way.
- [x] JSON naming snake_case, consistent with the codebase.
- [x] No denormalization — the per-credential verdict is derived from two
      fields, never stored per pair.

**API design**
- [x] No output shape changes; new data rides on `domain.Target`.
- [x] CLI and MCP stay symmetric.
- [x] Request/response examples included.

**Quality**
- [x] Edge cases listed — fourteen, including the mode asymmetry and pagination.
- [x] Happy-path and edge-case testing criteria, mapped to level.
- [x] Dependencies listed.
- [x] Unproven area named, with an explicit instruction to validate the central
      assumption against a real cluster before merging.

**Verified while writing, not assumed** — the previous plan flagged these as
"from memory, verify before writing the spec":
- `list-access-entries` returns a flat array of principal ARNs and paginates via
  `nextToken`.
- `authenticationMode` has **three** values, not two. The earlier framing
  ("`CONFIG_MAP` → unknown") was incomplete: `API_AND_CONFIG_MAP` also makes a
  *negative* unknowable while leaving a positive trustworthy.
- SSO access entries carry a full IAM path; the earlier assumption that stripping
  `assumed-role/…/session` to `role/<name>` would match was wrong for exactly the
  SSO case this targets.

**Open risk, not resolved here**: case 7 (same role name under different paths)
is an accepted false positive. If that shape exists in the target fleet, matching
must become path-aware and the spec needs revisiting.
