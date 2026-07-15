package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// inventoryCmd groups the read-only views over discovered state — the
// low-level model entities operators inspect occasionally. The things they act
// on (targets, credentials) stay top-level; user-owned organization
// (collections, current) stays top-level too. Discovered inventory only.
func (a *app) inventoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Inspect discovered access sources, scopes, and providers",
	}
	cmd.AddCommand(
		a.inventorySourcesCmd(),
		a.inventoryScopesCmd(),
		a.inventoryProvidersCmd(),
	)
	return cmd
}

func (a *app) inventorySourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "List discovered access sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sources, err := services.NewSourceService(a.store).List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, sources)
			}
			if len(sources) == 0 {
				fprintln(out, "No sources. Run `kuberoutectl sync <provider>` first.")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "ID\tPROVIDER\tNAME\tKIND")
			for _, s := range sources {
				fprintln(tw, string(s.ID)+"\t"+string(s.ProviderID)+"\t"+s.Name+"\t"+s.Kind)
			}
			return tw.Flush()
		},
	}
}

func (a *app) inventoryScopesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scopes",
		Short: "List discovered scopes (e.g. subscriptions, accounts, projects)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scopes, err := services.NewScopeService(a.store).List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, scopes)
			}
			if len(scopes) == 0 {
				fprintln(out, "No scopes. Run `kuberoutectl sync <provider>` first.")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "ID\tPROVIDER\tKIND\tNAME")
			for _, s := range scopes {
				fprintln(tw, string(s.ID)+"\t"+string(s.ProviderID)+"\t"+s.Kind+"\t"+s.Name)
			}
			return tw.Flush()
		},
	}
}

// providerView is the render-friendly projection of a provider and its
// capabilities.
type providerView struct {
	ID           domain.ProviderID   `json:"id"`
	Capabilities domain.Capabilities `json:"capabilities"`
}

func (a *app) inventoryProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
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
