package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/mcpserver"
)

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

// TestMCPSchemaListsEveryProvider guards the hand-written provider
// enumerations in the MCP tools' `jsonschema` struct tags — the same drift that
// shipped in v1.1.0 on `target list --help`, which said "(azure|aws)" long
// after gcp and kubeconfig were registered.
//
// Those tags cannot be derived from the registry the way the CLI help now is:
// struct tags are compile-time constants. So they are guarded instead. The
// expected set comes from the real wired registry via newApp(), so a fifth
// provider fails this test without anyone remembering to update it.
//
// It lives in package cli rather than mcpserver because mcpserver receives its
// registry as a dependency and never wires the real providers, while newApp()
// does. Reaching the other way would invert the layering (mcpserver's own tests
// depending on the CLI wiring layer), and with this repo's convention of
// internal `package mcpserver` test files it would also be an import cycle,
// since cli imports mcpserver.
//
// The comparison is set equality against the parenthesised `(a|b|c)` group, not
// a substring search: a future id that happens to be a substring of one already
// listed (say "gc" alongside "gcp") would satisfy strings.Contains without ever
// appearing, which is exactly the false negative this test exists to prevent.
// Equality also catches the reverse — an id left behind after a provider is
// removed.
func TestMCPSchemaListsEveryProvider(t *testing.T) {
	a, err := newApp()
	if err != nil {
		t.Fatalf("newApp(): %v", err)
	}
	var ids []string
	for _, p := range a.registry.List() {
		ids = append(ids, string(p.ID()))
	}
	if len(ids) < 2 {
		t.Fatalf("expected several registered providers, got %v", ids)
	}

	// Every MCP tool input that takes a provider id. Add new ones here.
	inputs := []reflect.Type{
		reflect.TypeFor[mcpserver.ListTargetsInput](),
		reflect.TypeFor[mcpserver.SyncProviderInput](),
	}
	for _, rt := range inputs {
		field, ok := fieldByJSONName(rt, "provider")
		if !ok {
			t.Errorf("%s: no field with json name %q", rt.Name(), "provider")
			continue
		}
		desc := field.Tag.Get("jsonschema")
		if desc == "" {
			t.Errorf("%s.%s: empty jsonschema tag", rt.Name(), field.Name)
			continue
		}
		listed, ok := pipeGroup(desc)
		if !ok {
			t.Errorf("%s.%s: jsonschema %q has no (a|b|c) provider group",
				rt.Name(), field.Name, desc)
			continue
		}
		if !slices.Equal(listed, slices.Sorted(slices.Values(ids))) {
			t.Errorf("%s.%s: jsonschema lists %v, registry has %v (tag: %q)",
				rt.Name(), field.Name, listed, ids, desc)
		}
	}
}

// pipeGroup extracts the first parenthesised pipe-separated group from a
// jsonschema description — "...id (azure|aws|gcp)..." yields [azure aws gcp],
// sorted. Reports false when no such group is present.
func pipeGroup(desc string) ([]string, bool) {
	_, rest, ok := strings.Cut(desc, "(")
	if !ok {
		return nil, false
	}
	inner, _, ok := strings.Cut(rest, ")")
	if !ok {
		return nil, false
	}
	parts := strings.Split(inner, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return slices.Sorted(slices.Values(parts)), true
}

// fieldByJSONName finds the struct field whose `json` tag names it n, ignoring
// options like ",omitempty".
func fieldByJSONName(rt reflect.Type, n string) (reflect.StructField, bool) {
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == n {
			return f, true
		}
	}
	return reflect.StructField{}, false
}
