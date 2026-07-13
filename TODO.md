# TODO

Implementation tracker for the `kuberoutectl` milestone-1 MVP (Azure + AWS).
See `ARCHITECTURE.md` for the design and `AGENTS.md` for standing rules.

## Slice 1 — Spine (no cloud)  ✅ done

Provider-agnostic backbone, fully testable without any cloud CLI.

- [x] `go.mod` (`github.com/ymedlop/kuberoutectl`, entrypoint `cmd/kuberoutectl`)
- [x] `internal/domain` — Provider capabilities, AccessSource, Credential,
      Scope, Target (system vs user labels), AccessHealth, ActionHint,
      LabelSelector, Collection, InventorySnapshot, Selection
- [x] Label validation + reserved `kuberoutectl.io/` namespace enforcement
- [x] `internal/providers` — Provider interface + compile-time Registry
- [x] `internal/cache` + `jsonstore` — atomic JSON persistence, cache/ vs state/ split
- [x] `internal/execx` — CommandRunner, BinaryResolver (config→managed→PATH→error), FakeRunner
- [x] `internal/services` — DoctorService
- [x] `internal/cli` — Cobra root with `--output text|json`, `provider list`, `doctor`
- [x] Tests: registration, JSON round-trip (+ user-label preservation),
      binary resolution precedence, selector eval, label validation, doctor

## Slice 2 — Azure provider + inventory read  ✅ done

- [x] `internal/providers/azure` driver + capabilities (CanRenew, CanReauth, CanDiscoverScopes)
- [x] Discovery via `az`: login/account state → Credentials, subscriptions → Scopes, AKS → Targets
- [x] Pure `parse.go` + `build.go` over captured `az` JSON fixtures (`testdata/`)
- [x] Health mapping (token expiry → valid/expiring/expired, epoch preferred) + ActionHint
- [x] Graceful logged-out path (single expired credential hinting renew, no error)
- [x] Renew orchestration (`az login [--tenant]`)
- [x] `DiscoveryService` with user-label re-attach + per-provider replace-only merge
- [x] `SourceService`, `ScopeService`, `CredentialService`, `TargetService`
- [x] CLI: `sync azure`, `source list`, `scope list`, `credential list/show/renew`, `target list/inspect`
- [x] Register azure in `cli.Execute` wiring point; doctor checks `az`
- [x] Tests: parse, health mapping, full discovery + logged-out (FakeRunner),
      label preservation across resync, provider-scoped merge, renew capability gate

## Slice 3 — Labels & collections (provider-agnostic)  ✅ done

- [x] `LabelService` — add/remove/list user labels, reserved-namespace enforcement,
      persists to state + keeps snapshot copy consistent (no resync needed)
- [x] CLI: `target label add/remove/list`
- [x] `SelectorEngine` + `ParseSelector` (equality, comma-joined, `key in [..]`)
- [x] Bare structured aliases (region/platform/provider/health/kind) selectable,
      matching README `region in [..]`; user labels override aliases
- [x] `CollectionService` — create/list/show/resolve (selector ∪ static, dedup), delete
- [x] CLI: `collection create --selector/--static/--description`, `list/show/use/delete`
- [x] `target use` + `collection use` via `SelectionService`, `Selection` persistence
- [x] Tests: selector parse + eval, label mutation/persistence/reserved-namespace,
      collection resolution + auto-join on resync + static union/dedup
- [x] E2E verified: user labels survive resync; collections re-resolve live

## Slice 4 — AWS provider  ✅ done

- [x] `internal/providers/aws` driver + capabilities (per-profile, auth-type aware,
      StaticCredentials=true to signal non-renewable profiles exist)
- [x] Discovery via `aws`: profiles → Sources, STS identity → Credentials,
      account → Scopes (deduped), EKS list+describe → Targets (per profile region)
- [x] Pure `parse.go` over captured `aws` fixtures (SSO, static-user, role identities)
- [x] Auth classification (sso/static/role/unknown) + health mapping
      (STS validity; static keys → static/none; SSO/role failure → expired/renew;
      static failure → error/manual)
- [x] Renew orchestration: `aws sso login` for sso/role; static/unknown refused with guidance
- [x] CLI: `sync aws` (auto-registered from registry); doctor checks `aws`
- [x] Register aws at wiring point
- [x] Tests: parse, classify, health matrix, full 3-profile discovery, renew refusal + sso path
- [x] E2E verified alongside Azure: mixed AKS+EKS inventory, static-vs-renewable
      credentials, cross-provider collection (`env=prod` spanning both clouds),
      `platform in [aks,eks]` multi-cloud view, resync survival

## Cross-cutting / release

- [ ] `Makefile` + GoReleaser wiring in `.github/workflows/snapshot-release.yml`
      (Windows amd64 primary, then Linux amd64)
- [ ] Extend `README.md` usage docs with real command output
- [x] `credential renew`, `target use`, `collection use` end-to-end
- [ ] Multi-region EKS scan (currently the profile's configured region only)
- [ ] Managed-runtime resolution (step 2) — optional, deferred

## Future providers (post-MVP)

- [ ] kubeconfig provider — static credentials, context switching, no renewal
- [ ] GCP provider — gcloud, projects → Scopes, GKE → Targets
- [ ] Richer selector semantics beyond exact-match / in-list
