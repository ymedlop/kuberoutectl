package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/buildinfo"
	"github.com/ymedlop/kuberoutectl/internal/mcpserver"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// mcpCmd runs an MCP (Model Context Protocol) server over stdio, exposing a
// small, safe set of kuberoutectl capabilities as structured tools for AI
// clients. It is opt-in: nothing listens unless this command runs.
//
// stdout is the MCP transport channel, so this handler must never write to
// stdout — the server keeps protocol frames there, and any diagnostics go to
// stderr.
func (a *app) mcpCmd() *cobra.Command {
	var readOnly bool
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP server exposing kuberoutectl as tools (stdio)",
		Long: "Serve a Model Context Protocol server over stdio so an MCP-compatible AI\n" +
			"client can list inventory, inspect credential health, route targets, and\n" +
			"manage collections. No secrets and no destructive actions are exposed.\n" +
			"Use --read-only to expose the inspection tools only.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			deps := mcpserver.Deps{
				Version:     buildinfo.Version,
				Registry:    a.registry,
				Discovery:   services.NewDiscoveryService(a.registry, a.store, nil),
				Sources:     services.NewSourceService(a.store),
				Scopes:      services.NewScopeService(a.store),
				Credentials: services.NewCredentialService(a.store, a.registry),
				Targets:     services.NewTargetService(a.store),
				Selection:   services.NewSelectionService(a.store, a.registry, nil),
				Collections: services.NewCollectionService(a.store, services.NewSelectorEngine()),
			}
			return mcpserver.Serve(cmd.Context(), deps, readOnly)
		},
	}
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "expose only read tools (no sync/use/collection writes)")
	return cmd
}
