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
		a.targetHideCmd(),
		a.targetUnhideCmd(),
	)
	return cmd
}

func (a *app) targetListCmd() *cobra.Command {
	var (
		provider  string
		selectors []string
		wide      bool
		all       bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered Kubernetes targets",
		Long: "List discovered Kubernetes targets.\n\n" +
			"Filter with --provider (" + a.providerIDs() + ") and/or --selector (repeatable),\n" +
			"e.g. `--selector env=prod` or `--selector \"region in [westeurope]\"`.\n" +
			"Hidden targets are omitted by default; pass --all to include them, or\n" +
			"`--selector hidden=true` to list only hidden ones.\n" +
			"The ALIAS column is a short handle you can pass to `target use`,\n" +
			"`target inspect`, and `target label` instead of the full ID.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter := services.TargetFilter{Provider: domain.ProviderID(provider), IncludeHidden: all}
			if len(selectors) > 0 {
				sel, err := services.ParseSelector(selectors)
				if err != nil {
					return err
				}
				filter.Selector = &sel
			}
			// One call, one snapshot read: deriving the rows and the profile
			// names from the same result keeps them from describing two
			// different generations if a `sync` lands mid-command.
			rows, err := services.NewTargetService(a.store, a.registry).ListWithCredentials(filter)
			if err != nil {
				return err
			}
			targets := make([]domain.Target, 0, len(rows))
			for _, r := range rows {
				targets = append(targets, r.Target)
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, targets)
			}
			if len(targets) == 0 {
				fprintln(out, "No targets. Run `kuberoutectl sync <provider>` first.")
				return nil
			}
			// Show the HIDDEN column only when hidden targets are actually in view
			// (via --all or a visibility selector), so the default listing stays clean.
			anyHidden := false
			for _, t := range targets {
				if t.Hidden {
					anyHidden = true
					break
				}
			}
			// Same rule for PROFILES: only worth a column when some target really
			// has a choice of access path. The names come from a live join, not
			// from a denormalized field on the target.
			profiles := make(map[domain.TargetID][]string, len(rows))
			anyMulti := false
			for _, r := range rows {
				names := r.CredentialNames()
				profiles[r.Target.ID] = names
				if len(names) > 1 {
					anyMulti = true
				}
			}
			tw := newTabWriter(out)
			header := "ALIAS\tPLATFORM\tVERSION\tREGION\tHEALTH\tPROVIDER"
			if anyMulti {
				header += "\tPROFILES"
			}
			if anyHidden {
				header += "\tHIDDEN"
			}
			if wide {
				header += "\tID"
			}
			fprintln(tw, header)
			for _, t := range targets {
				// Same fallback `inspect` uses: a target cached before versions were
				// tracked has an empty field, and a blank cell in a table reads as a
				// value rather than as an absence.
				version := t.KubernetesVersion
				if version == "" {
					version = domain.VersionUnknown
				}
				row := t.Alias + "\t" + t.Platform + "\t" + version + "\t" + t.Region + "\t" + string(t.Health) + "\t" + string(t.ProviderID)
				if anyMulti {
					row += "\t" + strings.Join(profiles[t.ID], ",")
				}
				if anyHidden {
					mark := ""
					if t.Hidden {
						mark = "yes"
					}
					row += "\t" + mark
				}
				if wide {
					row += "\t" + string(t.ID)
				}
				fprintln(tw, row)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "filter by provider ("+a.providerIDs()+")")
	cmd.Flags().StringArrayVarP(&selectors, "selector", "l", nil, "filter by label selector (repeatable)")
	cmd.Flags().BoolVarP(&wide, "wide", "w", false, "also show the full target ID")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "include hidden targets")
	return cmd
}

func (a *app) targetInspectCmd() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "inspect <alias|id|name>",
		Short: "Show a single target in detail, including labels",
		Long: "Show a single target in detail, including labels.\n\n" +
			"Operability comes from the last `sync`. Pass --refresh to re-establish it\n" +
			"against the provider instead — one extra API call, for the cluster named.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := services.NewTargetService(a.store, a.registry)
			// Either path, never both: ResolveWithAccessCheck calls
			// ResolveWithCredentials itself, so running both loaded the snapshot
			// twice for one command.
			joined, err := services.TargetWithCredentials{}, error(nil)
			if refresh {
				joined, err = svc.ResolveWithAccessCheck(cmd.Context(), args[0])
			} else {
				joined, err = svc.ResolveWithCredentials(args[0])
			}
			if err != nil {
				return err
			}
			if joined.AccessReason != "" {
				fprintln(cmd.ErrOrStderr(), "Note:", joined.AccessReason)
			}
			target := joined.Target
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				// Deliberately the bare target, not the join: wrapping it would
				// change this command's JSON shape for anything already parsing
				// it. credential_ids is additive and ships; per-credential health
				// is available from `credential list`.
				return renderJSON(out, target)
			}
			tw := newTabWriter(out)
			fprintln(tw, "Alias\t"+target.Alias)
			fprintln(tw, "ID\t"+string(target.ID))
			fprintln(tw, "Name\t"+target.Name)
			fprintln(tw, "Platform\t"+target.Platform)
			fprintln(tw, "Region\t"+target.Region)
			fprintln(tw, "Endpoint\t"+target.Endpoint)
			// A target cached before versions were tracked has an empty field;
			// render it as unknown so the value is never blank at display time.
			version := target.KubernetesVersion
			if version == "" {
				version = domain.VersionUnknown
			}
			fprintln(tw, "Version\t"+version)
			fprintln(tw, "Health\t"+string(target.Health))
			fprintln(tw, "Action\t"+string(target.ActionHint))
			fprintln(tw, "Scope\t"+string(target.ScopeID))
			fprintln(tw, "Credential\t"+string(target.CredentialID))
			// Only worth listing when there is a choice. Health per credential is
			// joined live from the snapshot, so an expired alternative shows as
			// expired here even though the target itself reports the primary's
			// health.
			// Only printed when a check actually ran, so a target nobody had to
			// disambiguate reads exactly as it did before this existed.
			if target.AccessCheck != "" {
				fprintln(tw, "Access check\t"+describeAccessCheck(target.AccessCheck))
			}
			if len(joined.Credentials) > 1 {
				for i, c := range joined.Credentials {
					mark := ""
					if i == 0 {
						mark = "  (primary)"
					}
					// The verdict sits next to health because they answer different
					// questions — health is whether the profile can authenticate to
					// AWS at all, operability whether the cluster admits it.
					verdict := "  " + string(target.CredentialAccess(c.ID))
					fprintln(tw, "profile\t"+c.Name+"  "+string(c.Health)+"  "+string(c.ActionHint)+verdict+mark)
				}
			}
			for k, v := range target.SystemLabels {
				fprintln(tw, "system-label\t"+k+"="+v)
			}
			for k, v := range target.UserLabels {
				fprintln(tw, "user-label\t"+k+"="+v)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "re-check operability against the provider instead of using the last sync")
	return cmd
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
			target, err := services.NewTargetService(a.store, a.registry).Resolve(args[0])
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
			target, err := services.NewTargetService(a.store, a.registry).Resolve(args[0])
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
			target, err := services.NewTargetService(a.store, a.registry).Resolve(args[0])
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
			removed, err := services.NewTargetService(a.store, a.registry).Delete(args[0])
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
			svc := services.NewTargetService(a.store, a.registry)
			targets, err := svc.List(services.TargetFilter{})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(targets) == 0 {
				fprintln(out, "No targets to clear.")
				return nil
			}
			if !yes && !confirmPrompt(cmd, fmt.Sprintf("Delete all %d target(s)?", len(targets))) {
				fprintln(out, "Aborted.")
				return nil
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

func (a *app) targetHideCmd() *cobra.Command {
	var selectors []string
	cmd := &cobra.Command{
		Use:   "hide [<alias|id|name>]",
		Short: "Hide targets from the default list (persists across resyncs)",
		Long: "Hide one target by ref, or many with --selector. Hidden targets are\n" +
			"remembered in user state and stay hidden across resyncs. They still\n" +
			"appear under `target list --all` (and `--selector hidden=true`), and can\n" +
			"be revealed again with `target unhide`.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.toggleVisibility(cmd, args, selectors, true)
		},
	}
	cmd.Flags().StringArrayVarP(&selectors, "selector", "l", nil, "hide all targets matching this selector (repeatable)")
	return cmd
}

func (a *app) targetUnhideCmd() *cobra.Command {
	var selectors []string
	cmd := &cobra.Command{
		Use:   "unhide [<alias|id|name>]",
		Short: "Reveal previously hidden targets",
		Long: "Reveal one target by ref, or many with --selector (e.g.\n" +
			"`--selector hidden=true` to reveal everything currently hidden).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.toggleVisibility(cmd, args, selectors, false)
		},
	}
	cmd.Flags().StringArrayVarP(&selectors, "selector", "l", nil, "reveal all targets matching this selector (repeatable)")
	return cmd
}

// toggleVisibility hides or reveals targets, given exactly one of a ref arg or a
// --selector. It keeps the two commands' handlers thin.
func (a *app) toggleVisibility(cmd *cobra.Command, args, selectors []string, hide bool) error {
	vis := services.NewVisibilityService(a.store)
	out := cmd.OutOrStdout()
	verb := "Hid"
	if !hide {
		verb = "Revealed"
	}

	if len(selectors) > 0 {
		if len(args) > 0 {
			return fmt.Errorf("provide either a target ref or --selector, not both")
		}
		sel, err := services.ParseSelector(selectors)
		if err != nil {
			return err
		}
		var matched []domain.Target
		if hide {
			matched, err = vis.HideSelector(sel)
		} else {
			matched, err = vis.UnhideSelector(sel)
		}
		if err != nil {
			return err
		}
		fprintln(out, verb, len(matched), "target(s).")
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a target ref or --selector")
	}
	var (
		tgt domain.Target
		err error
	)
	if hide {
		tgt, err = vis.HideRef(args[0])
	} else {
		tgt, err = vis.UnhideRef(args[0])
	}
	if err != nil {
		return err
	}
	fprintln(out, verb+" target:", tgt.Alias, "("+tgt.Name+")")
	return nil
}

func (a *app) targetUseCmd() *cobra.Command {
	var (
		noKubeconfig bool
		profile      string
		refresh      bool
	)
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
			res, err := services.NewSelectionService(a.store, a.registry, nil).
				UseTarget(cmd.Context(), args[0], services.UseTargetOptions{
					Activate:       activate,
					CredentialName: profile,
					Refresh:        refresh,
				})
			if err != nil {
				return err
			}
			target := res.Target
			out := cmd.OutOrStdout()
			// On stderr, and ahead of the JSON branch, so it reaches the operator
			// whether or not stdout is being piped into something. It warns rather
			// than blocks: the verdict is from the last sync and may be stale, and
			// entering a cluster to diagnose exactly this is legitimate.
			if line := describeAccess(res, refresh); line != "" {
				fprintln(cmd.ErrOrStderr(), line)
			}
			if a.output == formatJSON {
				// The bare target, as this command has always rendered. Wrapping
				// it to add the credential would break the shape for anything
				// already parsing it — the same reasoning applied to `target
				// inspect`. Which credential was used is reported by
				// `current -o json`, whose selection object carries it.
				return renderJSON(out, target)
			}
			via := describeCredentialChoice(res)
			if activate {
				fprintln(out, "Now using target:", target.Alias, "("+target.Name+")"+via)
				fprintln(out, "kubeconfig updated and set as the current context.")
			} else {
				fprintln(out, "Recorded selection:", target.Alias, "("+target.Name+")"+via+" — kubeconfig unchanged.")
			}
			if target.ActionHint == domain.ActionRenew {
				fprintln(out, "Note: this target's credential needs renewal — run `kuberoutectl credential renew`.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noKubeconfig, "no-kubeconfig", false, "record the selection only; do not modify ~/.kube/config")
	cmd.Flags().StringVar(&profile, "profile", "", "go in through this credential (an AWS profile name) when several reach the target")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "re-check operability against the provider instead of using the last sync")
	return cmd
}

// describeAccess renders what is known about the credential being used, for
// stderr.
//
// One rule, and the flag does not change it: **say what is known, whatever it
// is.** Silence means "nothing was established" and nothing else — previously it
// also meant "admitted", "not checked" and "this provider has no such concept",
// which made it useless as a signal.
//
// What --refresh changes is freshness, and whether an inconclusive answer is
// explained. Explaining it unconditionally would print a line on every `target
// use` in a fleet where most clusters are inconclusive, and a message that
// always appears stops being read.
func describeAccess(res services.UseTargetResult, refreshed bool) string {
	switch {
	case res.AccessWarning != "":
		return "Warning: " + res.AccessWarning

	case res.AccessReason != "":
		// Set exactly when a --refresh could not run. The service deliberately
		// keeps the cached verdict in that case, so report it alongside rather
		// than replacing knowledge with a failure notice — the reader still has
		// an answer, just an older one.
		msg := "Could not check access entries: " + res.AccessReason
		switch res.AccessVerdict {
		case domain.AccessOperable:
			msg += " " + res.Credential.Name + " held one at the last sync."
		case domain.AccessNotOperable:
			msg += " " + res.Credential.Name + " held none at the last sync; kubectl may return Forbidden."
		}
		return msg

	case res.AccessVerdict == domain.AccessOperable:
		if refreshed {
			return res.Credential.Name + " holds an access entry on this cluster."
		}
		// Past tense on purpose: this is cache, and a positive read as current
		// is the one a reader acts on without checking.
		return res.Credential.Name + " held an access entry on this cluster at the last sync."

	case refreshed && res.AccessVerdict == domain.AccessUnknown:
		return describeAccessUnknown(res.Credential.Name, res.Target.AccessCheck)
	}
	return ""
}

// describeAccessUnknown says why a live check could not settle the question, or
// returns "" when there is genuinely nothing to say — a target whose provider
// has no access-entry concept at all, where explaining the absence of an answer
// would invent a subject.
func describeAccessUnknown(name string, mode domain.AccessCheckMode) string {
	switch mode {
	case domain.AccessCheckAPIAndConfigMap:
		return "Could not tell whether " + name + " can operate here: it holds no access entry, " +
			"and this cluster also honours aws-auth, which kuberoutectl does not read."
	case domain.AccessCheckConfigMap:
		return "Could not tell whether " + name + " can operate here: this cluster uses CONFIG_MAP " +
			"authentication, where access entries do not apply."
	default:
		return ""
	}
}

// describeAccessCheck explains a mode in the terms that matter to the reader:
// not what it is called, but whether an absence means anything.
func describeAccessCheck(m domain.AccessCheckMode) string {
	switch m {
	case domain.AccessCheckAPI:
		return string(m) + " (a profile absent from the list will be refused)"
	case domain.AccessCheckAPIAndConfigMap:
		return string(m) + " (aws-auth may also grant access, so only a listed profile is confirmed)"
	case domain.AccessCheckConfigMap:
		return string(m) + " (access entries do not apply to this cluster)"
	case domain.AccessCheckUnavailable:
		return string(m) + " (the check could not run — likely no eks:ListAccessEntries)"
	default:
		return string(m)
	}
}

// describeCredentialChoice renders how the access path was picked, or "" when
// the target has only one and there was nothing to pick.
//
// The default case is worded differently on purpose. For a cluster several
// profiles can reach, the primary is only the healthiest one — kuberoutectl
// cannot see EKS access entries, so being able to authenticate is not evidence
// of being able to operate. Presenting that guess in the same words as an
// explicit choice would hide the one fact the operator needs to act on.
func describeCredentialChoice(res services.UseTargetResult) string {
	name := res.Credential.Name
	if name == "" {
		return ""
	}
	switch res.CredentialSource {
	case services.CredentialFromFlag:
		return " via " + name
	case services.CredentialFromMemory:
		return " via " + name + " (remembered)"
	default:
		// A previous choice that the cache no longer offers must be reported
		// even though the target now has a single access path and looks like it
		// never had a choice. Staying silent here would move an operator off a
		// deliberately-picked profile with no output at all.
		if res.LostCredentialID != "" {
			return " via " + name + " (credential " + string(res.LostCredentialID) + " is gone from the cache)"
		}
		if len(res.Target.CredentialIDs) < 2 {
			return "" // only one way in: nothing was chosen, so say nothing
		}
		return " via " + name + " (default — pass --profile to pick another)"
	}
}
