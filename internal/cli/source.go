package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) sourceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Inspect discovered access sources"}
	cmd.AddCommand(a.sourceListCmd())
	return cmd
}

func (a *app) sourceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
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
