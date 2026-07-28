# Spec: live access check on `target use`

**Created**: 2026-07-28
**Status**: draft
**Author**: Yeray Medina López
**Epic**: none
**Target release**: v1.2.0
**Follows**: `.planning/specs/aws-access-entry-operability.md` (shipped as #112)

---

## Problem

#112 reads EKS access entries during `sync aws`, but only for clusters that
**more than one profile reaches** — the cost bound it set as an explicit goal,
on the reasoning that with one way in there is nothing to choose.

Field evidence says that reasoning was incomplete. In the operator's real fleet
almost every cluster is reached by a single profile, so the check almost never
runs and every target reports `unknown`. Manually querying
`aws eks list-access-entries` on those same clusters **does** return the
operator's principal. The information exists; kuberoutectl declines to fetch it.

The gap between the two cases is the word *choose*. With one profile there is
nothing to choose, but there is still something to **know**: whether the one way
in will be refused. That question has a natural moment — the operator has just
named a cluster and is about to enter it — and at that moment the check costs one
API call for one cluster rather than a sweep of the fleet.

The concrete failure today:

```
$ kuberoutectl target use eks-prod-x
Now using target: eks-prod-x (eks-prod-x)
$ kubectl get pods
Error from server (Forbidden): ...
```

Nothing in the inventory hinted at it, and #112's verdict said `unknown` because
no check was ever attempted.

## Goal

`target use` tells the operator whether the credential it is about to use is
admitted by the cluster — for any AWS target, not only ones with a choice of
profile — and never blocks an activation over it.

Verifiable:

1. `target use <aws-target>` performs one `list-access-entries` call for that
   cluster and reports the verdict, including for a single-profile target.
2. A **confirmed refusal** warns on stderr, names an admitted profile when one is
   known, and still writes the kubeconfig.
3. A **confirmed admission** is reported too — the operator asked "can I operate
   here", and silence is not an answer to that.
4. A check that fails (no `eks:ListAccessEntries`, throttled, offline) leaves the
   activation untouched and says it could not tell.
5. An **azure, gcp or kubeconfig** target performs no check and no extra call —
   asserted with a runner that fails the test if invoked.
6. `sync aws` behaviour is unchanged.

## User stories

- **As an SRE entering a cluster**, I want to know before `kubectl` does whether
  this profile is admitted, so a `Forbidden` is not how I find out.
- **As an operator driving an ops agent**, I want `use_target` to return that
  verdict, so the agent can pick a different cluster rather than fail halfway
  through an analysis it has already started.
- **As someone diagnosing a broken access setup**, I want the activation to
  happen anyway, because entering a cluster to investigate exactly this is a
  legitimate thing to do.

## Requirements

### Must have

1. **The check runs on every `target use` against an AWS target**, with or
   without `--profile`, and with or without `--no-kubeconfig` — the selection is
   being recorded either way, so the question applies either way.

2. **AWS only.** Access entries are an EKS mechanism. Azure and GCP have their
   own authorization models which kuberoutectl does not read, and kubeconfig
   targets have no provider to ask. Those targets keep today's behaviour exactly,
   with no extra call. This is expressed as an **optional provider interface**
   reached by type assertion — the same pattern `ContextActivator` and
   `CredentialActivator` already use — so the core stays provider-agnostic and no
   `switch provider` appears in a service.

3. **A failed check never blocks activation.** Refusing to write a kubeconfig
   because the EKS API could not be reached would lock the operator out of a
   cluster they may well have access to, at exactly the moment they are trying to
   diagnose it. Failure is reported, the activation proceeds.

4. **Both directions are reported**, on stderr:

   | Verdict | Output |
   |---|---|
   | operable | `ops holds an access entry on this cluster.` |
   | not operable | `Warning: ops has no access entry …; kubectl may return Forbidden.` + an admitted profile when known |
   | unknown | the reason, phrased so it does not read as a refusal |

   Reusing `domain.AccessVerdictFor` and `domain.AccessCheckMode` rather than a
   second truth table — only a *negative* may depend on the authentication mode,
   and that rule must exist in exactly one place.

5. **The authentication mode comes from the cached target**
   (`Metadata["authentication_mode"]`, written by #112's `buildTarget`), not from
   a second `describe-cluster`. Under `CONFIG_MAP` no call is made at all, since
   access entries do not apply. The mode is therefore as fresh as the last
   `sync`; that staleness is accepted and stated, because re-describing the
   cluster would double the cost to refine an answer that is `unknown` either
   way.

6. **The result rides on `UseTargetResult`**, so the CLI and the MCP
   `use_target` tool render the same verdict from the same service call and
   cannot drift — the property #112 already relies on for its warning.

### Nice to have

- `--no-access-check` to skip the call on a slow link.
- Writing the fresh verdict back into the snapshot, so a later `target list` or
  `inspect` reflects what `use` learned.

### Out of scope

- **Changing `sync aws`.** Its check still drives primary selection for
  multi-profile clusters, which is the one thing a live check at `use` time
  cannot do — by then the primary has already been chosen. The two are
  complementary, and this spec adds nothing to discovery.
- **Updating the cache from `target use`.** Discovery owns the snapshot;
  write-on-read introduces concurrency questions this feature does not need.
  Listed as nice-to-have above instead.
- **Reading `aws-auth`**, `kubectl auth can-i` probing, or creating entries — all
  rejected in #112's spec for reasons that have not changed.
- Extending any of this to Azure, GCP or kubeconfig.

## Data model

**No new persisted fields.** `UseTargetResult` gains an in-memory result:

```go
// AccessCheck is what a live check established about the credential in use.
// Zero value means no check was attempted — a non-AWS target, or a provider
// that does not implement the optional interface.
type AccessCheckResult struct {
    Attempted bool
    Mode      domain.AccessCheckMode
    Verdict   domain.AccessVerdict
    // Reason is set when Verdict is AccessUnknown, phrased for display.
    Reason string
    // Alternatives names credentials the cluster is known to admit, for the
    // refusal case. Empty when none is known.
    Alternatives []string
}
```

`domain.Target.AccessCheck` and `OperableCredentialIDs` are untouched: they hold
what the last **sync** established, and this feature deliberately does not write
to the cache.

## API changes

New optional provider interface, beside `ContextActivator` / `CredentialActivator`:

```go
// AccessChecker is implemented by providers that can tell whether a credential
// is admitted *inside* a target, as opposed to able to authenticate to the
// provider. Optional: a provider without a Kubernetes-side authorization layer
// it can read simply does not implement it.
type AccessChecker interface {
    CheckAccess(ctx context.Context, t domain.Target, cred domain.Credential) (AccessCheck, error)
}

type AccessCheck struct {
    Mode   domain.AccessCheckMode
    Listed bool
}
```

The verdict is derived by the service via `domain.AccessVerdictFor(Listed, Mode)`,
so the provider reports facts and the domain owns the rule.

CLI: no new flags, no output shape change — one extra stderr line.

MCP `use_target` output, additive:

```json
{ "profile": "ops", "profile_source": "flag",
  "access_verdict": "operable",
  "access_check": "api_and_config_map" }
```

`access_warning` keeps its current meaning (set only on a confirmed refusal), so
existing consumers are unaffected.

## UI changes

```console
$ kuberoutectl target use eks-prod-frankfurt
Fetching credentials into ~/.kube/config ...
ops holds an access entry on this cluster.
Now using target: eks-prod-frankfurt (eks-prod-frankfurt)
kubeconfig updated and set as the current context.

$ kuberoutectl target use eks-prod-frankfurt --profile prod-sso
Warning: prod-sso has no access entry on this cluster; kubectl may return
         Forbidden. ops does have one.
Now using target: eks-prod-frankfurt (eks-prod-frankfurt) via prod-sso

$ kuberoutectl target use eks-prod-madrid
Could not check access entries (profile ops may lack eks:ListAccessEntries).
Now using target: eks-prod-madrid (eks-prod-madrid)
```

## Edge cases

1. **Single-profile cluster** — checked. This is the whole point; #112 skips it.
2. **`CONFIG_MAP` cluster** — no call, and the line says entries do not apply.
   Must never read as a refusal.
3. **`API_AND_CONFIG_MAP`, principal absent** — `unknown`, not a warning. The
   common case in a real fleet, so a false alarm here would be constant.
4. **Non-AWS target** — no call at all, asserted on the runner.
5. **`eks:ListAccessEntries` denied** — reported, activation proceeds.
6. **Malformed response on a successful call** — the activation still proceeds,
   but the check reports it could not tell. This is *not* the hard-error case of
   #112: there, a parse failure during discovery could poison the whole
   inventory; here it affects one line of output on one command, and failing an
   activation over it would be disproportionate. The difference is stated because
   the two look alike.
7. **Pagination** — `nextToken` followed, same as #112. A truncated page reads as
   an absent principal, which under `API` is a confirmed refusal.
8. **An identity with no ARN** — no key to match, so `unknown`, never a refusal.
9. **`--no-kubeconfig`** — checked and reported; the selection is still recorded.
10. **A target whose cached mode is missing** (synced before #112) — treated as
    unreadable: no call, `unknown`, with a line saying a `sync` would refresh it.
11. **The check is slow** — bounded by the provider CLI's own behaviour, as every
    other call in this tool is. No new timeout policy is invented here.

## Testing criteria

**Happy path**

- AWS target, mode `API`, credential listed → `operable`, reported, kubeconfig
  written.
- Same target, credential absent → warning naming an admitted profile,
  kubeconfig still written.

**Edge cases**

- One FakeRunner test per case 1–10, with case 4 asserting on `runner.Calls`
  rather than on output — a result-level assertion passes just as well when the
  call was made and discarded.
- The verdict derivation is **not** re-tested here; it is `domain`'s table from
  #112 and duplicating it would let the two drift.
- CLI: the three renderings, and that a refusal goes to **stderr** while the
  selection line stays on stdout.
- MCP: `access_verdict` present and equal to what the CLI rendered, from one
  service call.
- e2e: `target use` against the fake `aws`, asserting the line appears for an
  AWS target and no `list-access-entries` call is made for the azure one.

**Stated gap**: as with #112, no test runs against a real EKS cluster. The ARN
matching this relies on was validated manually against the operator's account
after #112 shipped — positives matched — so the reduction is no longer an
unvalidated assumption, but the live path here is fixture-driven.

## Dependencies

- `internal/providers/aws`: `checkAccessEntries`, `principalKey`,
  `parseAccessEntries` all exist from #112 and are reused rather than
  reimplemented.
- `domain.AccessVerdictFor`, `domain.AccessCheckMode` — the single truth table.
- `internal/services.SelectionService.UseTarget` — extended.
- No new module, no new external binary, no new persisted state.

---

## Gate 1

**Completeness**
- [x] Problem stated with field evidence and the failing sequence, including why
      #112's cost bound was right for sync and wrong here.
- [x] Goal measurable — six outcomes, two of them negative (no call for non-AWS,
      sync unchanged).
- [x] Three user stories: the human, the agent, the person debugging.
- [x] Requirements split must / nice / out of scope.
- [x] Out of scope names what is deliberately *not* changed and why.

**Data model**
- [x] Nothing persisted; the cached fields keep meaning "what sync established".
- [x] The reason for not writing back is stated rather than assumed.

**API design**
- [x] Optional provider interface, matching the existing extension pattern.
- [x] The provider reports facts; the domain owns the verdict rule, so the truth
      table exists once.
- [x] MCP stays symmetric with the CLI, from one service call.
- [x] No CLI flag added, so no `make docs-reference` churn is expected — to be
      confirmed in the plan, not assumed.

**Quality**
- [x] Eleven edge cases, including the one that looks like #112's hard-error rule
      but must behave differently, with the difference argued.
- [x] Testing criteria per level, with the negative case asserted on calls.
- [x] The unproven area named, and the part that is now *proven* (ARN matching,
      validated against a real account) distinguished from it.

**Open question for Gate 2**: whether `CheckAccess` should take the credential at
all, or take the target and return the full admitted set. The latter is one call
either way and would let a single check answer for every profile reaching the
target — useful for `inspect`, unnecessary for `use`. Deciding it needs the
plan's view of whether `inspect` should also go live.

**Gate 1: PASSED**
