# Spec: AWS multi-profile targets

**Created**: 2026-07-27
**Status**: draft
**Author**: Yeray Medina López
**Epic**: none
**Target release**: 1.2.0

## Problem

`internal/providers/aws/build.go:70` derives a target's identity from the EKS
cluster ARN. That ARN is **account-scoped, not profile-scoped**, so when two AWS
profiles authenticate into the same account and both see the same cluster,
discovery emits **two `domain.Target` entries carrying an identical `ID`**.
`mergeProviderResult` does not deduplicate within a provider, so both survive
into the snapshot.

The failure is silent rather than merely noisy:

- `AssignAliases` (`internal/services/alias.go:36`) disambiguates a duplicated
  name slug with `shortHash(t.ID)`. Identical IDs produce an **identical alias**.
- `ResolveTargetRef` returns the first match on the ID path and on the alias
  path; its "ambiguous" branch only fires for a name lookup.
- So the cluster is listed twice, the two rows are indistinguishable, and the
  second profile's target is **unreachable by any reference the CLI prints**.

Discovery itself is not broken — `Discover` already iterates every profile and
emits an `AccessSource` and a `Credential` per profile. What is missing is a
representation for "one cluster, several ways in", and any way to pick which
one `target use` should take.

## Goal

One target per physical cluster, which knows every credential that can reach it,
and an explicit operator choice of which one to use — persisted, so `current`
reports how you are actually authenticated.

Success is verifiable:

1. Two profiles into the same account seeing the same cluster produce **exactly
   one** target, whose ID is unchanged from today (the ARN).
2. `target list` shows that target once; `target inspect` breaks down the health
   of every profile that reaches it.
3. `target use <ref> --profile <name>` writes the kubeconfig via that profile,
   and a later `target use <ref>` with no flag reuses it.
4. No existing user label, static collection membership, hidden flag, or
   selection is invalidated by the upgrade.

## User stories

- As an SRE with a read-only `dev` profile and a break-glass `ops` profile into
  the same AWS account, I want to see each cluster **once** with both profiles
  listed, so my inventory reflects the fleet rather than my credential count.

- As that same SRE, I want `target use prod-eks --profile ops` to fetch the
  kubeconfig through the elevated profile, so I can escalate deliberately
  instead of editing `~/.kube/config` by hand.

- As an operator returning the next morning, I want `current` to tell me **which
  profile** my kubeconfig was written with, so I know whether I am still holding
  break-glass access.

## Requirements

### Must have

**Domain**

- `domain.Target` gains `CredentialIDs []CredentialID` — every credential that
  reaches this target, **primary first**. `CredentialID` (singular) is retained
  and always equals `CredentialIDs[0]`, so every existing reader keeps working.
  Per-profile health is a **join against `snapshot.Credentials`**, not duplicated
  onto the target: no new struct, no second copy of health to drift.
- `domain.Selection` gains `CredentialID CredentialID` (`omitempty`) recording
  which access path the current selection was activated through.
- Both fields are **provider-agnostic by construction**. Only the AWS provider
  populates more than one entry today; nothing in `services` branches on
  provider id.

**AWS provider**

- After enumerating profiles, `Discover` folds its per-(profile, cluster)
  targets by ARN into one target per cluster.
- **Primary selection**: the credential with the best health wins
  (`valid` > `expiring` > `static` > `expired` > `unknown`); ties break
  alphabetically by profile name. The fold is deterministic and independent of
  profile iteration order.
- The folded target **is** the primary candidate's own struct with
  `CredentialIDs` added — not a target assembled from parts. This keeps
  `Health`, `ActionHint`, `CredentialID`, `Metadata` and `SystemLabels`
  internally consistent by construction, as they are today.
  *(Tightened after Gate 3.5; see D4 in the plan for the selector bug the
  original wording allowed.)*
- Profile names for display are joined from `snapshot.Credentials` at read time.
  No comma-joined copy is stored on the target — same rule as health, and for
  the same reason. *(Revised after Gate 3.5; see D5.)*

**Selection**

- `SelectionService.UseTarget` accepts an optional `domain.CredentialID`. When
  empty it resolves in this order: the credential remembered in the persisted
  selection (if it is still among the target's `CredentialIDs`), else the
  primary.
- The resolved credential is passed to activation and persisted with the
  selection. Persistence happens **only after a requested activation succeeds** —
  the existing rule, unchanged.
- Activation uses the chosen credential's profile for
  `aws eks update-kubeconfig --profile`.

**CLI**

- `target use <ref> --profile <name>` — `<name>` is matched against
  `Credential.Name` among the target's `CredentialIDs`. The AWS provider already
  sets `Credential.Name = profile` (`build.go:31`), so the flag is a friendly
  spelling of "which credential, by name" and introduces no AWS conditional into
  `services`.
- `target list` gains a `PROFILES` column, populated from
  `Metadata["profiles"]`; empty for single-credential targets.
- `target inspect` lists every credential reaching the target with its own
  health and action hint, marking the primary.
- `current` shows the selected profile when the selection carries one.

**MCP**

- `UseTargetInput` gains `Profile string` with the same semantics, so an MCP
  client is not restricted to the primary. This closes the same CLI/MCP
  asymmetry that produced the v1.1.1 fix.

**Labels**

- New system label `kuberoutectl.io/credential`, carrying the **primary**
  credential's name, so `--selector kuberoutectl.io/credential=ops` is possible.

**Errors and invalidation**

- `--profile <name>` naming a profile that is not among the target's
  `CredentialIDs` fails before any external CLI runs, listing the profiles that
  are valid for that target.
- `--profile` against a target with a single credential fails with the same
  message rather than being silently ignored.
- A remembered credential that a later resync no longer reports is treated as
  absent: `current` says so, and `target use` with no flag falls back to the
  primary instead of failing on a phantom profile.

### Nice to have

- `target inspect -o json` exposing the per-credential breakdown as a nested
  array rather than requiring the caller to join against `list_credentials`.

### Out of scope

- **Multi-valued selector matching.** The system label holds the primary only,
  so `--selector kuberoutectl.io/credential=ops` means "ops is primary", not
  "reachable via ops". Making the selector engine handle set membership is a
  change to the selector grammar and belongs in its own spec. This limitation is
  documented, not hidden.
- **Interactive profile picker.** `--profile` is explicit; no TUI prompt.
- **Applying the same fold to other providers.** Azure has the analogous shape
  (one subscription reachable by several logins) but has not been observed to
  collide; the domain fields are generic so it can be added later without a
  domain change.
- **Per-profile renewal orchestration.** `credential renew` keeps operating on
  one credential id, as it does today.
- **Cross-account profile assumption chains** (`source_profile` / role chaining
  beyond what `sts get-caller-identity` already resolves).

## Data model

Persisted JSON, `cache/` and `state/` unchanged in location.

`domain.Target` — additive:

```go
// CredentialIDs lists every credential that can reach this target, primary
// first. CredentialID equals CredentialIDs[0]; both are populated so readers
// written before multi-credential targets keep working. Providers that expose
// exactly one way in leave this nil.
CredentialIDs []CredentialID `json:"credential_ids,omitempty"`
```

`domain.Selection` — additive:

```go
// CredentialID records which of the target's access paths the selection was
// activated through. Empty means "the primary", which is also what every
// selection written before this field existed decodes to.
CredentialID CredentialID `json:"credential_id,omitempty"`
```

**Migration**: both fields are `omitempty` and absent-means-primary, so an
existing cache decodes correctly with no migration step and no version bump of
the snapshot format. This is the reason the ARN stays the target ID: user
labels, static collection membership, the hidden set, and the current selection
are all keyed by target ID and would otherwise be orphaned.

## API changes

CLI surface:

```
kuberoutectl target use <ref> [--profile <name>] [--no-kubeconfig]
```

MCP `use_target` input, additive:

```json
{ "ref": "prod-eks", "activate": true, "profile": "ops" }
```

`current -o json`, additive:

```json
{ "selection": { "target_id": "arn:aws:eks:...:cluster/prod-eks",
                 "credential_id": "aws:ops",
                 "updated_at": "2026-07-27T10:00:00Z" } }
```

## UI changes

```
$ kuberoutectl target list
ALIAS      NAME       PROVIDER  REGION      PROFILES   HEALTH
prod-eks   prod-eks   aws       eu-west-1   ops, dev   valid

$ kuberoutectl target inspect prod-eks
...
Credentials:
  ops   aws:ops   valid    use     (primary)
  dev   aws:dev   expired  renew

$ kuberoutectl target use prod-eks --profile ops
Fetching credentials into ~/.kube/config ...
Now using target: prod-eks (prod-eks) via profile "ops"

$ kuberoutectl current
Target:  prod-eks (prod-eks)
Profile: ops
```

## Edge cases

1. **Same account, same cluster, two profiles** — the core case. Folds to one
   target with two `CredentialIDs`. Regression test asserts exactly one target
   and that the ID is the bare ARN.
2. **Same account, profiles configured for different regions** — each profile
   lists a different cluster set. The ARN encodes the region, so no fold occurs
   and each cluster is reachable only via the profile whose region matches.
   Falls out of folding by ARN; asserted so it stays that way.
3. **Primary expired, secondary valid** — the fold must pick the valid one, so
   `target list` reports `valid`. Verifies that health ordering, not profile
   ordering, drives the choice.
4. **Every profile for a cluster expired** — health `expired`, action `renew`,
   and the target's `CredentialID` points at the alphabetically-first profile so
   `credential renew` has an unambiguous subject.
5. **Remembered profile removed from `~/.aws/config`** — the resync drops that
   credential; `current` reports the selection's credential as no longer
   present, and `target use <ref>` with no flag activates the primary rather
   than failing.
6. **`--profile` naming a profile that cannot reach this cluster** — rejected
   before `aws` is invoked, listing the target's valid profiles. Covers both a
   nonexistent profile and a real profile scoped to another account.
7. **`--profile` on a single-credential target** (gcp, kubeconfig, azure) —
   same rejection path, same message shape. No silent no-op.
8. **Pre-upgrade cache** — a snapshot with no `credential_ids` and a selection
   with no `credential_id` must load, list, and activate exactly as before.
9. **Determinism** — the same fixture set, with profiles fed in any order,
   yields byte-identical `-o json` output. Guards the fold's tie-breaks.
10. **Kubeconfig overlay suppression is preserved** — `suppressOverlayDuplicates`
    matches by normalized endpoint. One target per cluster keeps that working;
    the rejected "one target per (cluster, profile)" model would have presented
    two native targets sharing an endpoint and confused it. Asserted explicitly
    so a future refactor cannot regress it unnoticed.
11. **A profile whose STS check fails** still yields a credential (today's
    resilient behaviour) but contributes no targets, so it must not appear in any
    target's `CredentialIDs`.

## Testing criteria

**Happy path**

- AWS fixture with two profiles, one account, one shared cluster → one target,
  `CredentialIDs` = `[ops dev]`, `CredentialID` = `aws:ops`.
- `target use ref --profile ops` → FakeRunner records
  `aws eks update-kubeconfig --name … --region … --profile ops`, and the
  persisted selection carries `credential_id: aws:ops`.
- `target use ref` with no flag, after the above → same profile, no flag needed.
- MCP `use_target` with `profile: "ops"` → same activation and same persisted
  selection as the CLI path.

**Edge cases**

- One test per numbered case above. Cases 1–4, 9 and 11 are provider-level
  (`internal/providers/aws`, fixtures under `testdata/`, FakeRunner). Cases 5–8
  and 10 are service-level (`internal/services`).
- Case 8 uses a checked-in snapshot fixture written in the pre-upgrade shape;
  it must not be regenerated by the new code, or it proves nothing.

**Ladder** — `go test ./... -count=1`, then `make check`, then
`bash scripts/e2e.sh`. The e2e fake `aws` gains a second profile over one
account so the fold is exercised against the **shipped binary**, not only unit
fixtures.

**Accepted gap, to be stated in the PR**: no test authenticates two real AWS
profiles against a real account. The fold, the flag, and the persistence are
fixture-driven; what stays unproven is that a live `aws configure list-profiles`
plus two live STS identities behave as the fixtures model them.

## Dependencies

- No new Go modules. No new external CLI calls — `aws eks update-kubeconfig`
  already takes `--profile` (`internal/providers/aws/activate.go:24`).
- Touches `internal/domain` (2 additive fields), `internal/providers/aws`
  (fold + build), `internal/services` (selection, inspect join, labels),
  `internal/cli` (flag, two columns, `current`), `internal/mcpserver`
  (one input field).
- Independent of `.planning/specs/update-check-notice.md`; both target 1.2.0 and
  do not overlap.

---

## Gate 1

**Completeness**
- [x] Problem clearly stated — located in code, with the silent-unreachability
      mechanism traced through alias assignment and ref resolution.
- [x] Goal specific and measurable — four numbered, checkable outcomes.
- [x] At least one user story — three.
- [x] Requirements split must-have / nice-to-have / out of scope.
- [x] Out of scope section exists — five items, each with its reason.

**Data model**
- [x] Column/field types correct — two additive fields, both `omitempty`.
- [x] Migration strategy defined — absent-means-primary; no snapshot version
      bump; the ARN is deliberately retained as the target ID to avoid orphaning
      labels, collections, hidden state and selection.
- [x] JSON field naming matches convention (snake_case).

**API design**
- [x] Flag and MCP field follow existing naming.
- [x] Request/response examples included.
- [x] CLI and MCP surfaces stay symmetric.

**Quality**
- [x] Edge cases listed — eleven.
- [x] Happy-path testing criteria.
- [x] Edge-case testing criteria, mapped to package and level.
- [x] Dependencies listed.
- [x] Unproven area named rather than implied.

**Known limitation carried forward, not resolved**: the new system label is
single-valued, so selectors can match "ops is primary" but not "reachable via
ops". Called out under Out of scope; worth revisiting at `/spartan:plan` if
selector-level filtering turns out to be the actual need.
