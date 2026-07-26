package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// rootTestApp builds a minimally-wired app: an empty-but-non-nil registry
// (enough to construct every command, since syncCmd lists the registry at build
// time) and a trace toggle (which PersistentPreRunE writes to, as newApp
// guarantees).
func rootTestApp() *app {
	return &app{registry: providers.NewRegistry(), output: formatText, trace: &execx.TraceConfig{}}
}

// testRoot builds the command tree from a minimally-wired app.
func testRoot() *cobra.Command {
	return rootTestApp().rootCmd()
}

// byName indexes a command's children by their name (first word of Use).
func byName(cmds []*cobra.Command) map[string]*cobra.Command {
	m := make(map[string]*cobra.Command, len(cmds))
	for _, c := range cmds {
		m[c.Name()] = c
	}
	return m
}

// TestRootCommand exercises the exported RootCommand()/newApp() path — real
// provider registration and its error returns — which cmd/gen-docs depends on.
// The plain rootCmd() tests below bypass that wiring.
func TestRootCommand(t *testing.T) {
	root, err := RootCommand()
	if err != nil {
		t.Fatalf("RootCommand() error: %v", err)
	}
	if root == nil {
		t.Fatal("RootCommand() returned nil")
	}
	if root.Name() != "kuberoutectl" {
		t.Errorf("root name = %q, want kuberoutectl", root.Name())
	}
	if _, ok := byName(root.Commands())["sync"]; !ok {
		t.Error("root is missing the sync command")
	}
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

// TestCompletionHidden confirms the auto-generated `completion` command is
// hidden from help but still present and usable (not disabled), so shell
// tab-completion keeps working.
func TestCompletionHidden(t *testing.T) {
	root := testRoot()
	root.InitDefaultCompletionCmd() // Cobra adds it lazily during Execute

	comp, ok := byName(root.Commands())["completion"]
	if !ok {
		t.Fatal("completion command should still exist (hidden, not disabled)")
	}
	if !comp.Hidden {
		t.Error("completion command should be hidden from the help/command list")
	}
}

// TestVerboseFlagWiring confirms the global --verbose/-v flag exists and that
// PersistentPreRunE flips the shared trace toggle: off by default, on (with a
// writer) when the flag is set.
func TestVerboseFlagWiring(t *testing.T) {
	if testRoot().PersistentFlags().Lookup("verbose") == nil {
		t.Fatal("root is missing the --verbose flag")
	}

	off := rootTestApp()
	rootOff := off.rootCmd()
	rootOff.SetOut(&bytes.Buffer{})
	rootOff.SetErr(&bytes.Buffer{})
	rootOff.SetArgs([]string{"version"})
	if err := rootOff.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if off.trace.Enabled {
		t.Error("tracing should be disabled without --verbose")
	}

	on := rootTestApp()
	rootOn := on.rootCmd()
	rootOn.SetOut(&bytes.Buffer{})
	rootOn.SetErr(&bytes.Buffer{})
	rootOn.SetArgs([]string{"--verbose", "version"})
	if err := rootOn.Execute(); err != nil {
		t.Fatalf("--verbose version: %v", err)
	}
	if !on.trace.Enabled {
		t.Error("--verbose should enable tracing")
	}
	if on.trace.Writer == nil {
		t.Error("--verbose should set the trace writer")
	}
}

// TestVersionFlagRich confirms `--version` prints the full build string
// (version + commit + date), matching the `version` subcommand rather than
// Cobra's bare default.
func TestVersionFlagRich(t *testing.T) {
	root := testRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(buf.String(), "commit") {
		t.Errorf("--version should include commit/date, got %q", buf.String())
	}
}

// TestProviderFlagHelpListsEveryProvider guards against the `--provider` help
// text going stale as providers are added. It shipped in v1.1.0 advertising
// only "(azure|aws)" while gcp and kubeconfig were both registered and both
// worked — the flag passes its value straight through with no whitelist, so the
// text was the only thing wrong, and nothing failed when it drifted.
//
// The expected set is not hardcoded: `sync` builds one subcommand per
// registered provider, so its children ARE the registry. Any provider added
// later is therefore covered without touching this test.
func TestProviderFlagHelpListsEveryProvider(t *testing.T) {
	root, err := RootCommand()
	if err != nil {
		t.Fatalf("RootCommand() error: %v", err)
	}
	top := byName(root.Commands())

	sync, ok := top["sync"]
	if !ok {
		t.Fatal("no sync command")
	}
	var providerIDs []string
	for _, c := range sync.Commands() {
		providerIDs = append(providerIDs, c.Name())
	}
	if len(providerIDs) < 2 {
		t.Fatalf("expected several registered providers, got %v", providerIDs)
	}

	for _, path := range [][2]string{{"target", "list"}, {"credential", "list"}} {
		parent, ok := top[path[0]]
		if !ok {
			t.Fatalf("no %s command", path[0])
		}
		leaf, ok := byName(parent.Commands())[path[1]]
		if !ok {
			t.Fatalf("no %s %s command", path[0], path[1])
		}
		flag := leaf.Flags().Lookup("provider")
		if flag == nil {
			t.Fatalf("%s %s has no --provider flag", path[0], path[1])
		}
		// Both the flag usage and the command's long help describe the filter;
		// v1.1.0 had the stale list in both places on `target list`.
		for _, text := range map[string]string{"flag usage": flag.Usage, "long help": leaf.Long} {
			if !strings.Contains(text, "provider") {
				continue // long help need not mention the filter at all
			}
			for _, id := range providerIDs {
				if !strings.Contains(text, id) {
					t.Errorf("%s %s: %q omits registered provider %q",
						path[0], path[1], text, id)
				}
			}
		}
	}
}
