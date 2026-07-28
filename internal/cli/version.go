package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/buildinfo"
	"github.com/ymedlop/kuberoutectl/internal/updatecheck"
)

// versionView is the render payload for `version -o json`. The update fields are
// omitempty and only ever populated by --check-update, so the default shape is
// unchanged for anything already parsing it.
type versionView struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`

	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
	// UpdateCheck explains why there is no verdict — offline, rate limited, a
	// non-release build. Present exactly when LatestVersion is not.
	UpdateCheck string `json:"update_check,omitempty"`
}

func (a *app) versionCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Long: "Print build version information.\n\n" +
			"With --check-update, also ask GitHub whether a newer stable release exists.\n" +
			"That request happens only when you ask for it: `version` on its own — and\n" +
			"every command except `doctor` — makes no network call of its own. Set\n" +
			updatecheck.EnvDisable + " to any value to disable the check everywhere.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			view := versionView{
				Version: buildinfo.Version,
				Commit:  buildinfo.Commit,
				Date:    buildinfo.Date,
			}

			// Shared with `doctor`, so the two commands cannot disagree about when
			// an update is worth reporting.
			var status *updateStatus
			if check && updatecheck.Enabled(a.version) {
				st := checkForUpdate(cmd.Context(), a.version, a.checker)
				status = &st
				if st.Newer {
					view.LatestVersion, view.UpdateAvailable = st.Latest, true
				} else {
					view.UpdateCheck = st.Detail
				}
			}

			if a.output == formatJSON {
				return renderJSON(out, view)
			}
			fprintln(out, "kuberoutectl", buildinfo.String())
			if status != nil {
				fprintln(out, status.Detail)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check-update", false, "also check whether a newer stable release is available")
	return cmd
}
