package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) collectionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "collection", Short: "Manage saved views over targets"}
	cmd.AddCommand(
		a.collectionCreateCmd(),
		a.collectionListCmd(),
		a.collectionShowCmd(),
		a.collectionUseCmd(),
		a.collectionDeleteCmd(),
	)
	return cmd
}

func (a *app) collectionCreateCmd() *cobra.Command {
	var selectors []string
	var staticIDs []string
	var description string
	cmd := &cobra.Command{
		Use:   "create <name> --selector <expr>",
		Short: "Create a collection from a selector and/or static targets",
		Long: "Create a saved view over targets.\n\n" +
			"Selectors accept key=value equalities (comma-joined or repeated) and\n" +
			"`key in [a, b]` in-lists, e.g.:\n" +
			"  --selector env=prod\n" +
			"  --selector env=prod,team=platform\n" +
			"  --selector \"region in [westeurope, eu-west-1]\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var sel domain.LabelSelector
			if len(selectors) > 0 {
				parsed, err := services.ParseSelector(selectors)
				if err != nil {
					return err
				}
				sel = parsed
			}
			ids := make([]domain.TargetID, 0, len(staticIDs))
			for _, id := range staticIDs {
				ids = append(ids, domain.TargetID(id))
			}
			col, err := services.NewCollectionService(a.store, nil).Create(args[0], description, sel, ids)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, col)
			}
			fprintln(out, "Created collection:", col.Name)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&selectors, "selector", nil, "selector clause (repeatable): key=value or `key in [a,b]`")
	cmd.Flags().StringArrayVar(&staticIDs, "static", nil, "explicit target ID to include (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	return cmd
}

func (a *app) collectionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cols, err := services.NewCollectionService(a.store, nil).List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, cols)
			}
			if len(cols) == 0 {
				fprintln(out, "No collections. Create one with `kuberoutectl collection create`.")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "NAME\tSTATIC\tDESCRIPTION")
			for _, c := range cols {
				fprintln(tw, c.Name+"\t"+itoa(len(c.StaticIDs))+"\t"+c.Description)
			}
			return tw.Flush()
		},
	}
}

// collectionShowView is the render payload for `collection show`: the
// definition plus its currently resolved members.
type collectionShowView struct {
	Collection domain.Collection `json:"collection"`
	Members    []domain.Target   `json:"members"`
}

func (a *app) collectionShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a collection's definition and its current members",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := services.NewCollectionService(a.store, nil)
			col, err := svc.Get(args[0])
			if err != nil {
				return err
			}
			members, err := svc.Resolve(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, collectionShowView{Collection: col, Members: members})
			}
			fprintln(out, "Collection:", col.Name)
			if col.Description != "" {
				fprintln(out, "Description:", col.Description)
			}
			fprintln(out, "Members:", itoa(len(members)))
			tw := newTabWriter(out)
			for _, m := range members {
				fprintln(tw, m.Name+"\t"+m.Platform+"\t"+m.Region+"\t"+string(m.Health))
			}
			return tw.Flush()
		},
	}
}

func (a *app) collectionUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Select a collection as the current view",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := services.NewCollectionService(a.store, nil)
			col, err := svc.Get(args[0])
			if err != nil {
				return err
			}
			members, err := svc.Resolve(args[0])
			if err != nil {
				return err
			}
			if err := services.NewSelectionService(a.store, a.registry, nil).UseCollection(col.ID); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, collectionShowView{Collection: col, Members: members})
			}
			fprintln(out, "Now using collection:", col.Name, "("+itoa(len(members))+" members)")
			return nil
		},
	}
}

func (a *app) collectionDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved collection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := services.NewCollectionService(a.store, nil).Delete(args[0]); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Deleted collection:", args[0])
			return nil
		},
	}
}
