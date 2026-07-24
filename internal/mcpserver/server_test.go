package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect runs srv over an in-memory transport and returns a connected client
// session; the server goroutine exits when the test's context is cancelled.
func connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverT, clientT := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func toolNames(t *testing.T, session *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	return got
}

var (
	readToolNames = []string{
		"list_providers", "list_targets", "get_target", "list_sources",
		"list_scopes", "list_credentials", "get_status", "list_collections",
		"get_collection",
	}
	writeToolNames = []string{"sync_provider", "use_target", "create_or_update_collection"}
)

func TestServer_RegistersFullToolSetAndRoundTrips(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	session := connect(t, New(h.d, false))

	got := toolNames(t, session)
	for _, name := range append(append([]string{}, readToolNames...), writeToolNames...) {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
	if len(got) != len(readToolNames)+len(writeToolNames) {
		t.Errorf("unexpected tool count: got %d, want %d", len(got), len(readToolNames)+len(writeToolNames))
	}

	// A real request/response over the transport.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_targets"})
	if err != nil {
		t.Fatalf("call list_targets: %v", err)
	}
	if res.IsError {
		t.Errorf("list_targets returned an error result: %+v", res.Content)
	}
}

// TestServer_ToolErrorSurfacesAsIsError proves a service error returned by a
// handler reaches the client as an MCP tool error, not a transport failure and
// not a successful-but-empty result (spec edge case 5).
func TestServer_ToolErrorSurfacesAsIsError(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	session := connect(t, New(h.d, false))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_target",
		Arguments: map[string]any{"ref": "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for an unknown target ref, got: %+v", res.Content)
	}
}

func TestServer_ReadOnlyOmitsWriteTools(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	session := connect(t, New(h.d, true))

	got := toolNames(t, session)
	if len(got) != len(readToolNames) {
		t.Errorf("read-only tool count: got %d, want %d", len(got), len(readToolNames))
	}
	for _, name := range writeToolNames {
		if got[name] {
			t.Errorf("read-only server exposed write tool %q", name)
		}
	}
}
