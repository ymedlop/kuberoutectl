package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/buildinfo"
)

// versionView is the render payload for `version -o json`.
type versionView struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, versionView{
					Version: buildinfo.Version,
					Commit:  buildinfo.Commit,
					Date:    buildinfo.Date,
				})
			}
			fprintln(out, "kuberoutectl", buildinfo.String())
			return nil
		},
	}
}
