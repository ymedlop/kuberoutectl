package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// syncSummary is the render-friendly result of a sync.
type syncSummary struct {
	Provider    domain.ProviderID `json:"provider"`
	Sources     int               `json:"sources"`
	Credentials int               `json:"credentials"`
	Scopes      int               `json:"scopes"`
	Targets     int               `json:"targets"`
}

func (a *app) syncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Discover inventory from a provider and update the local cache",
	}
	// One subcommand per registered provider keeps `sync azure`, `sync aws`
	// working without provider conditionals in the CLI.
	for _, p := range a.registry.List() {
		cmd.AddCommand(a.syncProviderCmd(p.ID()))
	}
	return cmd
}

func (a *app) syncProviderCmd(id domain.ProviderID) *cobra.Command {
	return &cobra.Command{
		Use:   string(id),
		Short: "Sync inventory from the " + string(id) + " provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			disco := services.NewDiscoveryService(a.registry, a.store, nil)
			snap, err := disco.Sync(cmd.Context(), id)
			if err != nil {
				return err
			}
			sum := syncSummary{
				Provider:    id,
				Sources:     countByProvider(snap, id, "source"),
				Credentials: countByProvider(snap, id, "credential"),
				Scopes:      countByProvider(snap, id, "scope"),
				Targets:     countByProvider(snap, id, "target"),
			}

			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, sum)
			}
			fprintln(out, "Synced provider:", string(id))
			fprintln(out, "  sources:    ", sum.Sources)
			fprintln(out, "  credentials:", sum.Credentials)
			fprintln(out, "  scopes:     ", sum.Scopes)
			fprintln(out, "  targets:    ", sum.Targets)
			return nil
		},
	}
}

// countByProvider counts entities of a kind belonging to a provider, so the
// summary reflects what this sync contributed rather than the whole cache.
func countByProvider(snap domain.InventorySnapshot, id domain.ProviderID, kind string) int {
	n := 0
	switch kind {
	case "source":
		for _, s := range snap.Sources {
			if s.ProviderID == id {
				n++
			}
		}
	case "credential":
		for _, c := range snap.Credentials {
			if c.ProviderID == id {
				n++
			}
		}
	case "scope":
		for _, s := range snap.Scopes {
			if s.ProviderID == id {
				n++
			}
		}
	case "target":
		for _, t := range snap.Targets {
			if t.ProviderID == id {
				n++
			}
		}
	}
	return n
}
