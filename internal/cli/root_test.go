package cli

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// testRoot builds the command tree with an empty-but-non-nil registry, enough
// to construct every command (syncCmd lists the registry at build time).
func testRoot() *cobra.Command {
	return (&app{registry: providers.NewRegistry(), output: formatText}).rootCmd()
}

// byName indexes a command's children by their name (first word of Use).
func byName(cmds []*cobra.Command) map[string]*cobra.Command {
	m := make(map[string]*cobra.Command, len(cmds))
	for _, c := range cmds {
		m[c.Name()] = c
	}
	return m
}

// TestRootCommandSurface locks the consolidated top-level command set: no
// provider is special at root (provider/source/scope/aws are gone), and the
// grouped `inventory`/`setup` parents are present.
func TestRootCommandSurface(t *testing.T) {
	top := byName(testRoot().Commands())

	for _, n := range []string{
		"sync", "target", "credential", "collection", "current",
		"inventory", "setup", "doctor", "version",
	} {
		if _, ok := top[n]; !ok {
			t.Errorf("missing top-level command %q", n)
		}
	}
	for _, n := range []string{"provider", "source", "scope", "aws"} {
		if _, ok := top[n]; ok {
			t.Errorf("%q must no longer be a top-level command", n)
		}
	}
}

// TestInventoryGroup checks the read-only model views moved under `inventory`.
func TestInventoryGroup(t *testing.T) {
	top := byName(testRoot().Commands())
	inv, ok := top["inventory"]
	if !ok {
		t.Fatal("no inventory command")
	}
	sub := byName(inv.Commands())
	for _, n := range []string{"sources", "scopes", "providers"} {
		if _, ok := sub[n]; !ok {
			t.Errorf("inventory missing subcommand %q", n)
		}
	}
}

// TestSetupGroup checks the AWS SSO helper moved under `setup aws-sso`.
func TestSetupGroup(t *testing.T) {
	top := byName(testRoot().Commands())
	setup, ok := top["setup"]
	if !ok {
		t.Fatal("no setup command")
	}
	if _, ok := byName(setup.Commands())["aws-sso"]; !ok {
		t.Error("setup missing aws-sso subcommand")
	}
}

// TestTargetAliases checks `clusters`/`cluster` route to the target command.
func TestTargetAliases(t *testing.T) {
	root := testRoot()
	tgt, ok := byName(root.Commands())["target"]
	if !ok {
		t.Fatal("no target command")
	}
	for _, alias := range []string{"clusters", "cluster"} {
		if !tgt.HasAlias(alias) {
			t.Errorf("target missing alias %q", alias)
		}
	}
	if c, _, err := root.Find([]string{"clusters", "list"}); err != nil || c.Name() != "list" {
		t.Errorf("`clusters list` did not resolve to target list (got %v, err %v)", c, err)
	}
}
