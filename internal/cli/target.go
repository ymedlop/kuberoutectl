package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) targetCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "target", Short: "Inspect, label, and use Kubernetes targets"}
	cmd.AddCommand(
		a.targetListCmd(),
		a.targetInspectCmd(),
		a.targetLabelCmd(),
		a.targetUseCmd(),
	)
	return cmd
}

func (a *app) targetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered Kubernetes targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targets, err := services.NewTargetService(a.store).List()
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
			fprintln(tw, "NAME\tPLATFORM\tREGION\tHEALTH\tPROVIDER\tID")
			for _, t := range targets {
				fprintln(tw, t.Name+"\t"+t.Platform+"\t"+t.Region+"\t"+string(t.Health)+"\t"+string(t.ProviderID)+"\t"+string(t.ID))
			}
			return tw.Flush()
		},
	}
}

func (a *app) targetInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show a single target in detail, including labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := services.NewTargetService(a.store).Get(domain.TargetID(args[0]))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, target)
			}
			tw := newTabWriter(out)
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
		Use:   "add <target-id> <key=value>",
		Short: "Add or update a user label on a target",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value, ok := strings.Cut(args[1], "=")
			if !ok {
				return fmt.Errorf("label must be key=value, got %q", args[1])
			}
			if err := services.NewLabelService(a.store).Add(domain.TargetID(args[0]), key, value); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Labeled", args[0], "with", args[1])
			return nil
		},
	}
}

func (a *app) targetLabelRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <target-id> <key>",
		Short: "Remove a user label from a target",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := services.NewLabelService(a.store).Remove(domain.TargetID(args[0]), args[1]); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Removed label", args[1], "from", args[0])
			return nil
		},
	}
}

func (a *app) targetLabelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <target-id>",
		Short: "List user labels on a target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := services.NewLabelService(a.store).List(domain.TargetID(args[0]))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, labels)
			}
			if len(labels) == 0 {
				fprintln(out, "No user labels on", args[0])
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

func (a *app) targetUseCmd() *cobra.Command {
	var noKubeconfig bool
	cmd := &cobra.Command{
		Use:   "use <id>",
		Short: "Select a target and fetch its credentials into ~/.kube/config",
		Long: "Select a target as current. By default this also fetches the cluster's\n" +
			"credentials into ~/.kube/config and sets it as the current kubectl context\n" +
			"(via the provider's native flow, e.g. az aks get-credentials /\n" +
			"aws eks update-kubeconfig). Use --no-kubeconfig to only record the selection.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			activate := !noKubeconfig
			if activate {
				fprintln(cmd.ErrOrStderr(), "Fetching credentials into ~/.kube/config ...")
			}
			target, err := services.NewSelectionService(a.store, a.registry, nil).
				UseTarget(cmd.Context(), domain.TargetID(args[0]), activate)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, target)
			}
			if activate {
				fprintln(out, "Now using target:", target.Name, "("+string(target.ID)+")")
				fprintln(out, "kubeconfig updated and set as the current context.")
			} else {
				fprintln(out, "Recorded selection:", target.Name, "("+string(target.ID)+") — kubeconfig unchanged.")
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
