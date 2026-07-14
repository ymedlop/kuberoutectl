package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local prerequisites (provider CLIs, resolution)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doctor := services.NewDoctorService(a.registry, a.resolver, a.requiredBinary)
			checks := doctor.Run()

			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, checks)
			}

			if len(checks) == 0 {
				fprintln(out, "No checks to run (no providers registered).")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "CHECK\tSTATUS\tDETAIL")
			for _, c := range checks {
				fprintln(tw, c.Name+"\t"+string(c.Status)+"\t"+c.Detail)
			}
			return tw.Flush()
		},
	}
}
