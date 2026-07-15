// Package cli is the Cobra adapter layer. Commands parse flags, call services,
// and render output — nothing else. Discovery, selector evaluation, label
// rules, and persistence all live behind services and must not leak in here.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/buildinfo"
	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/cache/jsonstore"
	"github.com/ymedlop/kuberoutectl/internal/config"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/providers/aws"
	"github.com/ymedlop/kuberoutectl/internal/providers/azure"
	"github.com/ymedlop/kuberoutectl/internal/providers/gcp"
	"github.com/ymedlop/kuberoutectl/internal/providers/kubeconfig"
)

// app bundles the wired-up dependencies shared across commands. It is built
// once in Execute and threaded through command constructors, keeping globals
// out of the CLI.
type app struct {
	cfg      config.Config
	registry *providers.Registry
	resolver execx.BinaryResolver
	store    cache.CacheStore
	// requiredBinary maps provider ID -> required CLI, used by doctor.
	requiredBinary map[string]string

	output outputFormat
}

// Execute builds the command tree and runs it. main.go calls this and nothing
// else.
func Execute() error {
	cfg := config.Default()
	a := &app{
		cfg:            cfg,
		registry:       providers.NewRegistry(),
		store:          jsonstore.New(cfg.CacheDir(), cfg.StateDir()),
		requiredBinary: map[string]string{},
		output:         formatText,
	}
	a.resolver = execx.NewPathResolver(a.cfg.BinaryPaths, "")
	runner := execx.NewExecRunner()

	// Providers register here — the single wiring point. Each provider also
	// declares the CLI doctor should check for it.
	if err := a.registry.Register(azure.New(a.resolver, runner)); err != nil {
		return err
	}
	a.requiredBinary[string(azure.ProviderID)] = azure.BinaryName

	if err := a.registry.Register(aws.New(a.resolver, runner)); err != nil {
		return err
	}
	a.requiredBinary[string(aws.ProviderID)] = aws.BinaryName

	if err := a.registry.Register(kubeconfig.New(a.resolver, runner)); err != nil {
		return err
	}
	a.requiredBinary[string(kubeconfig.ProviderID)] = kubeconfig.BinaryName

	if err := a.registry.Register(gcp.New(a.resolver, runner)); err != nil {
		return err
	}
	a.requiredBinary[string(gcp.ProviderID)] = gcp.BinaryName

	return a.rootCmd().Execute()
}

func (a *app) rootCmd() *cobra.Command {
	var output string
	root := &cobra.Command{
		Use:           "kuberoutectl",
		Short:         "Discover, organize, and route Kubernetes access across providers",
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			f := outputFormat(output)
			if !f.valid() {
				return fmt.Errorf("invalid --output %q: want text or json", output)
			}
			a.output = f
			return nil
		},
	}
	root.PersistentFlags().StringVarP(&output, "output", "o", "text", "output format: text|json")

	// Hide Cobra's auto-generated `completion` command from the help/command
	// list without disabling it — `kuberoutectl completion <shell>` and the
	// shell's dynamic tab-completion still work, they just don't clutter help.
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		a.syncCmd(),
		a.targetCmd(),
		a.credentialCmd(),
		a.collectionCmd(),
		a.currentCmd(),
		a.inventoryCmd(),
		a.setupCmd(),
		a.doctorCmd(),
		a.versionCmd(),
	)
	return root
}
