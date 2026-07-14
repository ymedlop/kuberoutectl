package cli

import (
	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

func (a *app) credentialCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "credential", Short: "Inspect and renew credentials"}
	cmd.AddCommand(a.credentialListCmd(), a.credentialShowCmd(), a.credentialRenewCmd())
	return cmd
}

func (a *app) credentialListCmd() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credentials and their health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := services.NewCredentialService(a.store, a.registry).List(domain.ProviderID(provider))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, creds)
			}
			if len(creds) == 0 {
				fprintln(out, "No credentials. Run `kuberoutectl sync <provider>` first.")
				return nil
			}
			tw := newTabWriter(out)
			fprintln(tw, "ID\tPROVIDER\tIDENTITY\tHEALTH\tACTION")
			for _, c := range creds {
				fprintln(tw, string(c.ID)+"\t"+string(c.ProviderID)+"\t"+c.Identity+"\t"+string(c.Health)+"\t"+string(c.ActionHint))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "filter by provider (azure|aws|gcp|kubeconfig)")
	return cmd
}

func (a *app) credentialShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a single credential in detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cred, err := services.NewCredentialService(a.store, a.registry).Get(domain.CredentialID(args[0]))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, cred)
			}
			tw := newTabWriter(out)
			fprintln(tw, "ID\t"+string(cred.ID))
			fprintln(tw, "Provider\t"+string(cred.ProviderID))
			fprintln(tw, "Identity\t"+cred.Identity)
			fprintln(tw, "Health\t"+string(cred.Health))
			fprintln(tw, "Action\t"+string(cred.ActionHint))
			if cred.ExpiresAt != nil {
				fprintln(tw, "ExpiresAt\t"+cred.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
			}
			return tw.Flush()
		},
	}
}

func (a *app) credentialRenewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "renew <id>",
		Short: "Renew or re-authenticate a credential (if the provider supports it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := services.NewCredentialService(a.store, a.registry)
			if err := svc.Renew(cmd.Context(), domain.CredentialID(args[0])); err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), "Renewed credential:", args[0])
			fprintln(cmd.OutOrStdout(), "Run `kuberoutectl sync` to refresh health.")
			return nil
		},
	}
}
