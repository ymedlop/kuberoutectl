package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the currently selected target or collection",
		Long: "Show what you are pointed at: the target or collection recorded by the\n" +
			"last `target use` / `collection use`, its health as of the last sync, and\n" +
			"how fresh that information is.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := services.NewSelectionService(a.store, a.registry, nil).Status()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, st)
			}

			if st.Selection.TargetID == "" && st.Selection.CollectionID == "" {
				fprintln(out, "Nothing selected. Run `kuberoutectl target use <alias>` or `kuberoutectl collection use <name>`.")
				return nil
			}

			tw := newTabWriter(out)
			switch {
			case st.Target != nil:
				fprintln(tw, "Target\t"+st.Target.Alias+" ("+st.Target.Name+")")
				fprintln(tw, "Provider\t"+string(st.Target.ProviderID))
				fprintln(tw, "Health\t"+string(st.Target.Health))
				fprintln(tw, "Action\t"+string(st.Target.ActionHint))
			case st.Selection.TargetID != "":
				fprintln(tw, "Target\t"+string(st.Selection.TargetID))
				fprintln(tw, "Status\tno longer in the cache — re-run `kuberoutectl sync` or pick another target")
			case st.Collection != nil:
				fprintln(tw, "Collection\t"+st.Collection.Name)
			default:
				fprintln(tw, "Collection\t"+string(st.Selection.CollectionID))
				fprintln(tw, "Status\tno longer exists — pick another collection")
			}
			fprintln(tw, "Selected\t"+humanAge(st.Selection.UpdatedAt)+" ago")
			if !st.SyncedAt.IsZero() {
				fprintln(tw, "Last sync\t"+humanAge(st.SyncedAt)+" ago")
			}
			return tw.Flush()
		},
	}
}

// humanAge renders the elapsed time since t coarsely (seconds, minutes, hours,
// or days) — enough to judge cache freshness without pretending precision.
func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	default:
		return itoa(int(d.Hours()/24)) + "d"
	}
}
