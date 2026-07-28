package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/services"
	"github.com/ymedlop/kuberoutectl/internal/updatecheck"
)

func (a *app) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local prerequisites (provider CLIs, resolution)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doctor := services.NewDoctorService(a.registry, a.resolver, a.requiredBinary)
			checks := doctor.Run()

			// Appended, never inserted: `doctor -o json` is documented as
			// machine-readable, so a consumer indexing the array must not find the
			// provider rows shifted underneath it.
			//
			// Enabled() is consulted before the checker is touched, which is what
			// makes "no row" and "no request" one decision rather than two that can
			// disagree.
			if updatecheck.Enabled(a.version) {
				checks = append(checks, updateCheckRow(cmd.Context(), a.version, a.checker))
			}

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
