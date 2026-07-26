// Package mcpserver exposes a subset of kuberoutectl's capabilities as Model
// Context Protocol (MCP) tools, so an MCP-compatible AI client can operate the
// CLI's safe workflows as structured tools instead of shelling out.
//
// It is a thin adapter: every tool handler decodes typed input, calls one
// existing service, and returns typed output. No business logic, no provider
// conditionals, no secrets, and no destructive actions live here — those
// concerns stay in the services and providers this package delegates to.
package mcpserver

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// Deps carries the wired services the tools delegate to. The CLI builds these
// from the same registry/store/resolver as every other command and hands them
// in; mcpserver never constructs its own persistence or providers.
type Deps struct {
	Version     string
	Registry    *providers.Registry
	Discovery   *services.DiscoveryService
	Sources     *services.SourceService
	Scopes      *services.ScopeService
	Credentials *services.CredentialService
	Targets     *services.TargetService
	Selection   *services.SelectionService
	Collections *services.CollectionService
}

// handler binds the deps to the tool methods and serializes the mutating tools.
// Reads are intentionally left unlocked: the JSON store writes to a temp file
// and renames over the target, so a concurrent load always sees a whole
// old-or-new file — the mutex only guards against lost updates from two
// overlapping load→mutate→save sequences (sync, use, collection upsert).
type handler struct {
	d  Deps
	mu sync.Mutex
}

// New builds an MCP server with the kuberoutectl tool set registered. When
// readOnly is true only the side-effect-free tools are registered, for handing
// a client an inspection-only connection.
func New(d Deps, readOnly bool) *mcp.Server {
	h := &handler{d: d}
	s := mcp.NewServer(&mcp.Implementation{Name: "kuberoutectl", Version: d.Version}, nil)
	h.registerReadTools(s)
	if !readOnly {
		h.registerWriteTools(s)
	}
	return s
}

// Serve builds the server and runs it over stdio until the client disconnects.
// stdout is the MCP transport channel, so the caller must keep every diagnostic
// on stderr.
func Serve(ctx context.Context, d Deps, readOnly bool) error {
	return New(d, readOnly).Run(ctx, &mcp.StdioTransport{})
}
