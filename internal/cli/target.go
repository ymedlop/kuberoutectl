package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) targetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "target",
		Aliases: []string{"clusters", "cluster"},
		Short:   "Inspect, label, and use Kubernetes targets",
	}
	cmd.AddCommand(
		a.targetListCmd(),
		a.targetInspectCmd(),
		a.targetLabelCmd(),
		a.targetUseCmd(),
		a.targetDeleteCmd(),
		a.targetClearCmd(),
	)
	return cmd
}

func (a *app) targetListCmd() *cobra.Command {
	var (
		provider  string
		selectors []string
		wide      bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered Kubernetes targets",
		Long: "List discovered Kubernetes targets.\n\n" +
			"Filter with --provider (azure|aws) and/or --selector (repeatable),\n" +
			"e.g. `--selector env=prod` or `--selector \"region in [westeurope]\"`.\n" +
			"The ALIAS column is a short handle you can pass to `target use`,\n" +
			"`target inspect`, and `target label` instead of the full ID.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter := services.TargetFilter{Provider: domain.ProviderID(provider)}
			if len(selectors) > 0 {
				sel, err := services.ParseSelector(selectors)
				if err != nil {
					return err
				}
				filter.Selector = &sel
			}
			targets, err := services.NewTargetService(a.store).List(filter)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, targets)
			}
			if len(targets) == 0 {
				fprintln(out, "No targets. Run `kuberoutectl sync <provider>` first.")
				return nil
			}
			tw := newTabWriter(out)
			header := "ALIAS\tPLATFORM\tREGION\tHEALTH\tPROVIDER"
			if wide {
				header += "\tID"
			}
			fprintln(tw, header)
			for _, t := range targets {
				row := t.Alias + "\t" + t.Platform + "\t" + t.Region + "\t" + string(t.Health) + "\t" + string(t.ProviderID)
				if wide {
					row += "\t" + string(t.ID)
				}
				fprintln(tw, row)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "filter by provider (azure|aws)")
	cmd.Flags().StringArrayVarP(&selectors, "selector", "l", nil, "filter by label selector (repeatable)")
	cmd.Flags().BoolVarP(&wide, "wide", "w", false, "also show the full target ID")
	return cmd
}

func (a *app) targetInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <alias|id|name>",
		Short: "Show a single target in detail, including labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := services.NewTargetService(a.store).Resolve(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, target)
			}
			tw := newTabWriter(out)
			fprintln(tw, "Alias\t"+target.Alias)
			fprintln(tw, "ID\t"+string(target.ID))
			fprintln(tw, "Name\t"+target.Name)
			fprintln(tw, "Platform\t"+target.Platform)
			fprintln(tw, "Region\t"+target.Region)
			fprintln(tw, "Endpoint\t"+target.Endpoint)
			fprintln(tw, "Health\t"+string(target.Health))
			fprintln(tw, "Action\t"+string(target.ActionHint))
			fprintln(tw, "Scope\t"+string(target.ScopeID))
			fprintln(tw, "Credential\t"+string(target.CredentialID))
			for k, v := range target.SystemLabels {
				fprintln(tw, "system-label\t"+k+"="+v)
			}
			for k, v := range target.UserLabels {
				fprintln(tw, "user-label\t"+k+"="+v)
			}
			return tw.Flush()
		},
	}
}

func (a *app) targetLabelCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "label", Short: "Manage user labels on a target"}
	cmd.AddCommand(a.targetLabelAddCmd(), a.targetLabelRemoveCmd(), a.targetLabelListCmd())
	return cmd
}

func (a *app) targetLabelAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <alias|id|name> <key=value>",
		Short: "Add or update a user label on a target",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value, ok := strings.Cut(args[1], "=")
			if !ok {
				return fmt.Errorf("label must be key=value, got %q", args[1])
			}
			target, err := services.NewTargetService(a.store).Resolve(args[0])
			if err != nil {
				return err
			}
			if err := services.NewLabelService(a.store).Add(target.ID, key, value); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Labeled", target.Alias, "with", args[1])
			return nil
		},
	}
}

func (a *app) targetLabelRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <alias|id|name> <key>",
		Short: "Remove a user label from a target",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := services.NewTargetService(a.store).Resolve(args[0])
			if err != nil {
				return err
			}
			if err := services.NewLabelService(a.store).Remove(target.ID, args[1]); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Removed label", args[1], "from", target.Alias)
			return nil
		},
	}
}

func (a *app) targetLabelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <alias|id|name>",
		Short: "List user labels on a target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := services.NewTargetService(a.store).Resolve(args[0])
			if err != nil {
				return err
			}
			labels, err := services.NewLabelService(a.store).List(target.ID)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, labels)
			}
			if len(labels) == 0 {
				fprintln(out, "No user labels on", target.Alias)
				return nil
			}
			tw := newTabWriter(out)
			for _, k := range sortedKeys(labels) {
				fprintln(tw, k+"\t"+labels[k])
			}
			return tw.Flush()
		},
	}
}

func (a *app) targetDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <alias|id|name>",
		Short: "Delete a target from the local cache",
		Long: "Delete a target from the local cache.\n\n" +
			"This is a cache cleanup, not a permanent exclusion: a later\n" +
			"`kuberoutectl sync <provider>` re-adds the target if the cluster still\n" +
			"exists. Scopes, credentials, and sources are left untouched.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := services.NewTargetService(a.store).Delete(args[0])
			if err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Deleted target:", removed.Alias, "("+removed.Name+")")
			return nil
		},
	}
}

func (a *app) targetClearCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete all targets from the local cache",
		Long: "Delete all targets from the local cache. Scopes, credentials, and sources\n" +
			"are kept, and a resync repopulates targets. Prompts for confirmation\n" +
			"unless --yes is given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc := services.NewTargetService(a.store)
			targets, err := svc.List(services.TargetFilter{})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(targets) == 0 {
				fprintln(out, "No targets to clear.")
				return nil
			}
			if !yes {
				ok, err := confirmPrompt(cmd, fmt.Sprintf("Delete all %d target(s)?", len(targets)))
				if err != nil {
					return err
				}
				if !ok {
					fprintln(out, "Aborted.")
					return nil
				}
			}
			n, err := svc.Clear()
			if err != nil {
				return err
			}
			fprintln(out, "Cleared", n, "target(s).")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func (a *app) targetUseCmd() *cobra.Command {
	var noKubeconfig bool
	cmd := &cobra.Command{
		Use:   "use <alias|id|name>",
		Short: "Select a target and fetch its credentials into ~/.kube/config",
		Long: "Select a target as current. The target can be given by its short alias\n" +
			"(see `target list`), its full ID, or its name. By default this also fetches\n" +
			"the cluster's credentials into ~/.kube/config and sets it as the current\n" +
			"kubectl context (via the provider's native flow, e.g. az aks get-credentials /\n" +
			"aws eks update-kubeconfig). Use --no-kubeconfig to only record the selection.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			activate := !noKubeconfig
			if activate {
				fprintln(cmd.ErrOrStderr(), "Fetching credentials into ~/.kube/config ...")
			}
			target, err := services.NewSelectionService(a.store, a.registry, nil).
				UseTarget(cmd.Context(), args[0], activate)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, target)
			}
			if activate {
				fprintln(out, "Now using target:", target.Alias, "("+target.Name+")")
				fprintln(out, "kubeconfig updated and set as the current context.")
			} else {
				fprintln(out, "Recorded selection:", target.Alias, "("+target.Name+") — kubeconfig unchanged.")
			}
			if target.ActionHint == domain.ActionRenew {
				fprintln(out, "Note: this target's credential needs renewal — run `kuberoutectl credential renew`.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noKubeconfig, "no-kubeconfig", false, "record the selection only; do not modify ~/.kube/config")
	return cmd
}
