# Plan: MCP server for kuberoutectl

**Spec**: .planning/specs/mcp-server.md
**Epic**: none
**Issue**: https://github.com/ymedlop/kuberoutectl/issues/44
**Created**: 2026-07-24
**Status**: shipped in #98
**Stack**: Go (Cobra CLI + layered services); new dep: official MCP Go SDK

---

## Verified SDK facts (used by this plan)

Official SDK: `github.com/modelcontextprotocol/go-sdk/mcp`, pin **v1.6.1** —
latest published stable, verified with `go list -m -versions` and
`go list -m ...@latest` (the earlier "v1.7.0" was a doc/spec-date confusion;
`v1.7.0` exists only as pre-releases and would fail to resolve). Re-verify at
build time. Confirmed API shape (present at v1.6.1):

```go
server := mcp.NewServer(&mcp.Implementation{Name: "kuberoutectl", Version: v}, nil)
mcp.AddTool(server, &mcp.Tool{Name: "list_targets", Description: "..."}, h.listTargets)
err := server.Run(ctx, &mcp.StdioTransport{})
```

Tool handler signature (typed in/out; SDK derives JSON Schema from `jsonschema`
struct tags):
```go
func (h *handler) listTargets(ctx context.Context, req *mcp.CallToolRequest, in ListTargetsInput) (*mcp.CallToolResult, ListTargetsOutput, error)
```

Handlers are plain methods → **unit-testable by direct call**, no transport
needed. An integration round-trip uses the SDK's in-memory transport pair
(`mcp.NewInMemoryTransports()`); if that helper's name differs at build time,
direct-handler tests remain the primary coverage.

---

## Architecture

A thin adapter package `internal/mcpserver` owns all MCP concerns; each tool
handler is ~5 lines that decode typed input → call **one existing service** →
return typed output. The `mcp` Cobra subcommand only wires services and serves.
No provider conditionals; no business logic outside services.

### Components

| Component | Type | Purpose |
|-----------|------|---------|
| `mcpserver.Deps` | struct | Carries the wired services + server version; passed in by the CLI. |
| `mcpserver.handler` | struct | Holds `Deps` + a `sync.Mutex` that serializes the 3 mutating tools (spec edge case 2). Reads are unlocked: `jsonstore` writes temp-file-then-renames (`jsonstore.go` writeJSON), so concurrent loads always see a whole old-or-new file — the mutex guards against lost updates from overlapping load→mutate→save, not read corruption. |
| `mcpserver.New(d, readOnly)` | constructor | Builds `*mcp.Server`, registers read tools always, write tools unless `readOnly`. |
| `mcpserver.Serve(ctx, d, readOnly)` | func | `New(...)` + `server.Run(ctx, &mcp.StdioTransport{})`. |
| read/write tool handlers | methods | One per tool; delegate to a service. |
| `mcpCmd` | Cobra command | `kuberoutectl mcp [--read-only]`: wire services from `app`, call `Serve`. |

### New files

| File | Location | Purpose |
|------|----------|---------|
| `server.go` | `internal/mcpserver/` | `Deps`, `handler`, `New`, `Serve`, tool registration. |
| `tools_read.go` | `internal/mcpserver/` | Read tool in/out structs + handlers (9 tools). |
| `tools_write.go` | `internal/mcpserver/` | Write tool structs + handlers (3 tools) + mutex use. |
| `tools_read_test.go` | `internal/mcpserver/` | Direct-handler tests on a seeded store + secret non-exposure. |
| `tools_write_test.go` | `internal/mcpserver/` | use_target/sync/upsert/serialization tests. |
| `server_test.go` | `internal/mcpserver/` | Registration set + in-memory round-trip. |
| `mcp.go` | `internal/cli/` | The `mcp` subcommand (wire + serve; `--read-only`). |
| `mcp_test.go` | `internal/cli/` | Subcommand present; builds without error. |

### Files to change

| File | What changes | Why |
|------|-------------|-----|
| `internal/services/collection.go` | Add `Save` (upsert): update in place if the name exists, else create. Keep `Create` as-is. | Back `create_or_update_collection`; keeps upsert logic in the service, not the tool. |
| `internal/services/collection_test.go` | Upsert test (create then update same name). | Cover the new method. |
| `internal/cli/root.go` | Add `a.mcpCmd()` to `root.AddCommand(...)`. | Register the subcommand. |
| `go.mod` / `go.sum` | Add `github.com/modelcontextprotocol/go-sdk` (pinned). | The MCP dependency (isolated to mcpserver + cli/mcp.go). |
| `docs/reference/*` | Regenerate via `make docs-reference`. | `mcp` page appears automatically. |
| `README.md` | One line: optional MCP server + how to launch. | Discoverability. |
| `docs/guides/mcp.md` (new) | Short guide: what it exposes, tool list, safety (no secrets/destructive), example client config. | User-facing docs. |
| `scripts/e2e.sh` | Best-effort MCP stdio smoke (initialize + tools/list + one read). | E2E proof; caveat if a real handshake is impractical in-script. |

### v1 tool → service map (from spec)

Read (9): `list_providers`→Registry; `list_targets`/`get_target`→TargetService;
`list_sources`→SourceService; `list_scopes`→ScopeService;
`list_credentials`→CredentialService; `get_status`→SelectionService
(supersedes a separate `get_current` — `Status()` already returns the same
`Selection`); `list_collections`/`get_collection`→CollectionService.
Write: `sync_provider`→DiscoveryService.Sync; `use_target`→SelectionService.UseTarget;
`create_or_update_collection`→CollectionService.Save.

---

## Tasks

### Phase 1: service upsert (independent)

| # | Task | Files |
|---|------|-------|
| 1 | `CollectionService.Save` upsert (update-in-place or create) + test. | `internal/services/collection.go`, `internal/services/collection_test.go` |

### Phase 2: MCP core + read tools (depends on the SDK dep)

| # | Task | Files |
|---|------|-------|
| 2 | Add SDK dep; `mcpserver` scaffold: `Deps`, `handler`, `New`/`Serve`, StdioTransport wiring, register one trivial tool to prove the build. | `go.mod`, `internal/mcpserver/server.go` |
| 3 | Read tool structs + handlers (9 tools), each delegating to its service; register in `New`. | `internal/mcpserver/tools_read.go`, `internal/mcpserver/server.go` |
| 4 | Read-tool tests (direct handler calls on a `jsonstore`-seeded `Deps`): `list_targets` shape, `get_status`, empty-cache empties, and **secret non-exposure** on `list_credentials`. | `internal/mcpserver/tools_read_test.go` |

### Phase 3: write tools (depends on Phase 2; task 5 also on Phase 1)

| # | Task | Files |
|---|------|-------|
| 5 | Write tool structs + handlers (`sync_provider`, `use_target`, `create_or_update_collection`) with `h.mu` serialization; register unless `readOnly`. | `internal/mcpserver/tools_write.go`, `internal/mcpserver/server.go` |
| 6 | Write-tool tests: `use_target` changes selection; `sync_provider` populates cache via a FakeRunner-backed provider; upsert via tool; two concurrent mutations don't corrupt the cache. | `internal/mcpserver/tools_write_test.go` |

### Phase 4: CLI wiring (depends on Phases 2–3)

| # | Task | Files |
|---|------|-------|
| 7 | `mcp` subcommand: build services from `app` (registry/store/resolver), `--read-only` flag, call `Serve`; stdout reserved for the transport (diagnostics → stderr); register in root. | `internal/cli/mcp.go`, `internal/cli/root.go` |
| 8 | Registration + round-trip test: `New` registers exactly the expected tool set (`--read-only` omits writes); in-memory client does `tools/list` + one read call. Plus `mcp` present on root. | `internal/mcpserver/server_test.go`, `internal/cli/mcp_test.go` |

### Phase 5: docs + e2e (depends on Phase 4; English)

| # | Task | Files |
|---|------|-------|
| 9 | `make docs-reference`; README line; new `docs/guides/mcp.md`. | `docs/reference/*`, `README.md`, `docs/guides/mcp.md` |
| 10 | e2e stdio smoke (best-effort). | `scripts/e2e.sh` |

### Parallel vs Sequential

| Parallel Group | Tasks | Why |
|---------------|-------|-----|
| Group A | Phase 1 (1) & Phase 2 (2) | Different packages; upsert is independent of MCP scaffold. |

| Sequential | Depends On | Why |
|-----------|-----------|-----|
| Task 3, 4 | Task 2 | Need the server scaffold + Deps. |
| Task 5 | Task 2, Task 1 | Write registration + collection upsert. |
| Task 6 | Task 5 | Tests the write handlers. |
| Task 7, 8 | Tasks 3, 5 | Wire + assert the full tool set. |
| Phase 5 | Task 7 | Reference regen needs the `mcp` command wired. |

---

## Testing Plan

| Test | Type | Spec tie |
|------|------|----------|
| `CollectionService.Save` create-then-update | service unit | Dependency / edge case 7 |
| `list_targets` returns seeded targets in documented shape | tool unit | Happy path |
| `get_status` reflects prior `use_target` | tool unit | Happy path |
| empty-cache read tools return empty arrays, no error | tool unit | Edge case 1 |
| `list_credentials` JSON has no token/secret/password/access_key key; matches `domain.Credential` field set | tool unit | Edge case 3 (no secrets) |
| `sync_provider` populates cache via FakeRunner | tool unit | Happy path |
| `use_target` missing ref → tool error | tool unit | Edge case 1 |
| upsert via `create_or_update_collection` twice | tool unit | Edge case 7 |
| two concurrent mutations don't corrupt cache (mutex) | tool unit | Edge case 2 |
| `New` registers expected tool set; `--read-only` omits writes | registration | Happy path / requirements |
| in-memory client `tools/list` + one read round-trip | integration | Happy path |
| `mcp` subcommand present on root | cli unit | Requirements |

Verification ladder: `go test ./...` → `make check` → `bash scripts/e2e.sh`.

---

## Gate 2 checklist

**Architecture**
- [x] Follows existing patterns (thin adapter over services, like `sync.go`; subcommand wires + delegates; no logic in Cobra handler).
- [x] Layering: cli → mcpserver → services → cache/providers; no upward calls; no provider conditionals.
- [x] New files in a dedicated `internal/mcpserver` package.

**Task Breakdown**
- [x] All changed files listed.
- [x] All new files listed with locations.
- [x] Each task ≤ 3 files / one commit.
- [x] Dependencies explicit (Group A parallel; write⇐scaffold+upsert; cli⇐tools; docs⇐cli).
- [x] Parallel vs sequential marked.

**Testing**
- [x] Data/service layer test (upsert).
- [x] Business-logic (tool handler) tests incl. secrets, empty cache, serialization.
- [x] Integration (registration + in-memory round-trip) + cli presence.
- [x] UI tests: N/A (no UI).
- [x] Spec edge cases 1,2,3,7 covered; 4 (partial sync) via sync_provider counts; 5 (schema) by SDK; 6 (activate) documented + exercised by use_target test.

**Gate 2: PASS**

---

## Notes / caveats (for the PR)

- First non-cobra runtime dependency; kept isolated to `internal/mcpserver` +
  `internal/cli/mcp.go`. Pin `v1.6.1` (latest stable, `go list`-verified).
- Full protocol conformance is delegated to the official SDK; our tests cover
  registration, handler behavior, secret non-exposure, and one live round-trip —
  not the entire MCP spec.
- The e2e stdio smoke is best-effort: a scripted `initialize`+`tools/list`
  handshake without a real client is fragile; if impractical, the in-memory
  integration test is the authoritative round-trip proof and the e2e step is
  stated as a caveat rather than faked.
- Build this on a fresh branch off `origin/development` (the verbose-tracing PR
  is still open/pending).
