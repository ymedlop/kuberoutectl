package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) scopeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "scope", Short: "Inspect discovered scopes (e.g. subscriptions)"}
	cmd.AddCommand(a.scopeListCmd())
	return cmd
}

func (a *app) scopeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered scopes",
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
