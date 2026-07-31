# Spec: MCP server for kuberoutectl

**Created**: 2026-07-24
**Status**: shipped in #98
**Author**: Yeray Medina López
**Epic**: none
**Issue**: https://github.com/ymedlop/kuberoutectl/issues/44

---

## Problem

AI clients that want to operate `kuberoutectl` on a user's behalf today have to
shell out to the CLI and scrape text, which is brittle and unsafe (easy to
invoke destructive commands, easy to leak output that wasn't meant to be
parsed). There is no structured, vendor-neutral way for a compatible AI client
to list inventory, inspect credential health, or route access through
`kuberoutectl` as typed tools.

## Goal

An **optional** MCP (Model Context Protocol) server, shipped as the
`kuberoutectl mcp` subcommand, that exposes a small, predictable set of tools
mapping to existing `kuberoutectl` **services** (not Cobra handlers). It is a
thin integration adapter: no new business logic, provider-agnostic, no secrets,
and no destructive actions in v1. Any MCP-compatible client can drive the safe
core workflows.

Success = a compatible MCP client, given the stdio server, can list providers,
inspect inventory/credentials/targets, see and change the current selection,
run a sync, and manage collections — with tool output structured and stable,
and no credential material ever returned.

---

## User Stories

- **As an AI client operating on behalf of an operator**, I connect to
  `kuberoutectl mcp` over stdio and call `list_targets` / `get_current` to
  understand what clusters exist and which is active, then `use_target` to route
  the operator's kubectl — without shelling out or parsing CLI text.
- **As an operator**, I enable the MCP server only when I want it (by running
  the subcommand), knowing it cannot delete targets, renew credentials, or
  reveal any secret, so handing an AI client the connection is low-risk.

---

## Requirements

### Must-have
- New subcommand `kuberoutectl mcp` that runs an MCP server over **stdio**
  (spawned as a subprocess by the client). The server is inert unless this
  command is invoked — MCP is opt-in and disabled by default.
- Implemented with the **official Go MCP SDK**
  (`github.com/modelcontextprotocol/go-sdk`), pinned to a specific version, for
  spec-compliant, vendor-neutral transport + typed tool schemas.
- A thin adapter package (`internal/mcpserver`) registers tools that delegate to
  existing services (`DiscoveryService`, `SourceService`, `ScopeService`,
  `CredentialService`, `TargetService`, `SelectionService`, `CollectionService`,
  provider `Registry`). **No business logic in the MCP layer.**
- The subcommand wires services from the same dependencies as the rest of the
  CLI (registry, cache store, resolver) and hands them to `mcpserver` — the
  Cobra handler only wires and serves, per the "no logic in handlers" rule.
- Tool output is structured JSON built from the existing `domain` types (the
  same shapes the CLI emits under `-o json`), for predictability.
- **No secrets**: credential tools return `domain.Credential` (which carries no
  token/key material — only identifiers, health, action, and non-secret
  metadata). A test asserts no secret-like fields leak.
- **No destructive actions in v1**: target delete/clear, credential renew,
  label add/remove, and hide/unhide are **not** registered.

#### v1 tool set

Read tools (side-effect free):

| Tool | Backing service call | Notes |
|------|----------------------|-------|
| `list_providers` | `Registry.List()` + `Capabilities()` | id + capability flags |
| `list_targets` | `TargetService.List(filter)` | optional filters: provider, label selector, include-hidden |
| `get_target` | `TargetService.Resolve(ref)` | ref = alias/id/name; includes k8s version, labels |
| `list_sources` | `SourceService.List()` | |
| `list_scopes` | `ScopeService.List()` | |
| `list_credentials` | `CredentialService.List(provider?)` | non-secret; optional provider filter |
| `get_status` | `SelectionService.Status()` | current selection (target/collection) + cache freshness; supersedes a separate `get_current` |
| `list_collections` | `CollectionService.List()` | |
| `get_collection` | `CollectionService.Resolve(name)` | resolved member targets |

Write tools (safe, non-destructive):

| Tool | Backing service call | Notes |
|------|----------------------|-------|
| `sync_provider` | `DiscoveryService.Sync(provider)` | runs provider discovery; returns counts (mirrors `sync`) |
| `use_target` | `SelectionService.UseTarget(ref, activate)` | sets current; `activate` merges kubeconfig context |
| `create_or_update_collection` | `CollectionService.Save(...)` (**new upsert**) | see dependency note — `Create` currently errors on an existing name |

### Nice-to-have
- `--read-only` flag on `kuberoutectl mcp` that registers only the read tools,
  for handing a client an inspection-only connection.
- `use_collection` (selection write, non-destructive) — deferred unless asked.

### Out of scope
- HTTP / SSE / streamable-HTTP transport (stdio only in v1; add later).
- Any destructive or interactive operation: `target delete`/`clear`,
  `credential renew` (interactive browser/device auth), label mutation,
  `hide`/`unhide`.
- Exposing secrets, tokens, or raw kubeconfig contents.
- AI-vendor-specific behavior, prompts, or tool naming.
- Authn/authz for the server (stdio inherits the local user's trust; a
  networked transport would need this and is out of scope).
- New product logic unrelated to the MCP surface.

---

## Data Model

No persistence or domain-type changes. Reuses the existing JSON cache/state via
the current services. One **service** addition is required (not a data-model
change): `CollectionService.Save` (upsert) to back `create_or_update_collection`
— see Dependencies.

---

## API (MCP tools) — request/response shape

Tools use typed input/output structs (the SDK generates JSON Schema from Go
struct tags). Examples:

`list_targets` input:
```json
{ "provider": "aws", "selector": "env=prod", "include_hidden": false }
```
`list_targets` output (array of the existing target JSON shape):
```json
{ "targets": [ { "id": "arn:aws:eks:...", "alias": "eks-prod-frankfurt",
  "platform": "eks", "region": "eu-central-1", "health": "valid",
  "provider_id": "aws", "kubernetes_version": "1.29", "labels": {"env":"prod"} } ] }
```

`use_target` input / output:
```json
{ "ref": "eks-prod-frankfurt", "activate": true }
→ { "target": { "id": "...", "alias": "eks-prod-frankfurt" }, "activated": true }
```

`sync_provider` output mirrors the CLI sync summary:
```json
{ "provider": "aws", "sources": 3, "credentials": 3, "scopes": 2, "targets": 2 }
```

`list_credentials` output (note: no secret fields — this is the whole shape):
```json
{ "credentials": [ { "id": "aws:default", "provider_id": "aws",
  "name": "default", "health": "expired", "action_hint": "renew" } ] }
```

Tool names are `snake_case`, provider-agnostic, and stable. Errors are returned
as MCP tool errors with a clear message (e.g. unknown target ref), distinct from
a successful-but-empty result.

---

## UI Changes

None (no CLI TUI). The only user-visible surface is the new `mcp` subcommand and
its `--help`, plus a docs page and README line.

---

## Edge Cases

1. **Empty cache (never synced)** — read tools return empty arrays, not errors.
   `use_target` with no matching ref returns a clear tool error.
2. **Long-lived server, interleaved calls** — unlike the one-shot CLI, the MCP
   server is a persistent process; a `sync_provider` write can interleave with
   reads. Mutating tools (`sync_provider`, `use_target`,
   `create_or_update_collection`) must be **serialized** (a server-level mutex)
   so cache writes don't race concurrent tool handlers.
3. **Secret non-exposure** — no tool returns token/key material; credential
   `metadata` is verified to hold only non-secret identifiers (profile,
   tenant_id, account, auth_type, resource_group). A test enforces this.
4. **Partial sync failure** (e.g. one profile's token expired) — `sync_provider`
   returns the summary counts like the CLI (resilient discovery), not a hard
   error; the provider's default-on diagnostic is surfaced in the tool result
   text, not as a tool failure.
5. **Unknown tool / bad params** — schema validation (from the SDK) rejects
   malformed input with a structured error; unknown tool names are rejected by
   the protocol layer.
6. **`use_target` with `activate=true`** — this merges a context into the user's
   kubeconfig (a real side effect on the local environment). It is in-scope and
   non-destructive (additive, `--overwrite-existing` semantics as today), but
   documented clearly; `activate=false` records selection only.
7. **`create_or_update_collection` on an existing name** — must upsert (update
   in place), since `CollectionService.Create` currently errors on duplicates.
8. **MCP SDK version** — latest published stable is **v1.6.1** (verified via
   `go list -m -versions`; `v1.7.0` exists only as pre-releases and would fail
   to resolve). Pin `v1.6.1`; re-verify at build time.

---

## Testing Criteria

### Happy path
- **Tool registration**: the server registers exactly the expected v1 tool set
  (names + input/output schemas present); `--read-only` registers only reads.
- **Request/response** via the SDK's in-memory/pipe transport: `list_targets`
  against a seeded store returns the seeded targets in the documented shape;
  `get_current` reflects a prior `use_target`.
- **`sync_provider`** against a FakeRunner-backed provider returns the expected
  counts and populates the cache.

### Edge cases
- **Secret non-exposure**: `list_credentials` output serialized to JSON contains
  no key named like `token`/`secret`/`password`/`access_key`, and matches the
  known `domain.Credential` field set.
- **Empty cache**: read tools return empty arrays without error; `use_target`
  on a missing ref returns a tool error.
- **Upsert**: `create_or_update_collection` twice with the same name updates
  rather than erroring.
- **Serialization**: concurrent mutating calls do not corrupt the cache
  (mutex-guarded) — a focused test driving two mutations.

### Verification ladder (CLAUDE.md)
- `go test ./...`
- `make check`
- `bash scripts/e2e.sh` — extend with a minimal MCP smoke test (spawn
  `kuberoutectl mcp`, send an `initialize` + `tools/list` + one read tool call
  over stdio, assert a structured response) if feasible without a real client.

---

## Dependencies

- **New Go module dependency**: `github.com/modelcontextprotocol/go-sdk`
  (official MCP Go SDK), pinned to **v1.6.1** (latest stable; verified via
  `go list`). This is the first non-cobra runtime dependency; keep it isolated
  to `internal/mcpserver` + `internal/cli/mcp.go`.
- **New service method**: `CollectionService.Save` / `Upsert` (create-or-update)
  in `internal/services/collection.go`, since `Create` errors on an existing
  name. Small, surgical, keeps upsert logic in the service (not the MCP layer),
  honoring "MCP does not duplicate business logic".
- Reuses all existing services + the `newApp` wiring (registry, `jsonstore`
  cache, `execx` resolver). No changes to the provider adapters or the domain
  model.
- Go 1.25 (already the module's version) — sufficient for the SDK.

---

## Design notes (for /spartan:plan)

- Package layout: `internal/mcpserver/` (server construction + tool
  registration + typed tool structs) and `internal/cli/mcp.go` (the subcommand
  that wires services and calls `mcpserver.Serve(ctx, stdio)`). Mirrors how
  `sync.go` wires `DiscoveryService`.
- Tool handlers are ~5 lines each: decode typed input → call one service method
  → return typed output. If a handler needs more than that, the logic belongs in
  a service, not the tool.
- Reuse `stderrProgress`-style stderr for any server logging so stdout stays a
  clean MCP transport channel (stdio uses stdout for protocol frames — server
  diagnostics MUST go to stderr).
- Provider-agnostic: no provider names hard-coded in tool schemas; `provider` is
  a free string filter validated against the registry.
