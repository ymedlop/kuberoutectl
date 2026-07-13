package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// providerView is the render-friendly projection of a provider and its
// capabilities.
type providerView struct {
	ID           domain.ProviderID   `json:"id"`
	Capabilities domain.Capabilities `json:"capabilities"`
}

func (a *app) providerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Inspect access providers",
	}
	cmd.AddCommand(a.providerListCmd())
	return cmd
}

func (a *app) providerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered providers and their capabilities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			views := make([]providerView, 0)
			for _, p := range a.registry.List() {
				views = append(views, providerView{ID: p.ID(), Capabilities: p.Capabilities()})
			}

			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, views)
			}

			if len(views) == 0 {
				fprintln(out, "No providers registered.")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "PROVIDER\tRENEW\tREAUTH\tSCOPES\tSWITCH\tSTATIC")
			for _, v := range views {
				c := v.Capabilities
				fprintln(tw,
					string(v.ID)+"\t"+
						yn(c.CanRenew)+"\t"+
						yn(c.CanReauth)+"\t"+
						yn(c.CanDiscoverScopes)+"\t"+
						yn(c.CanSwitchContext)+"\t"+
						yn(c.StaticCredentials))
			}
			return tw.Flush()
		},
	}
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
