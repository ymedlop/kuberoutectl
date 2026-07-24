package cli

import "testing"

// TestMCPCommandRegistered confirms the opt-in `mcp` subcommand is wired onto
// the root and carries its --read-only flag.
func TestMCPCommandRegistered(t *testing.T) {
	root := testRoot()
	mcp, ok := byName(root.Commands())["mcp"]
	if !ok {
		t.Fatal("root is missing the mcp command")
	}
	if mcp.Flags().Lookup("read-only") == nil {
		t.Error("mcp command is missing the --read-only flag")
	}
}
