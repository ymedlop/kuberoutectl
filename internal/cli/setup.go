package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/providers/aws"
)

// setupCmd groups machine-preparation helpers that write local provider config
// — the write-side counterpart to `doctor`'s read-only checks. Provider-specific
// setup lives here rather than under a per-provider root command, so no single
// provider is special at the top level.
func (a *app) setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Prepare local provider configuration",
	}
	cmd.AddCommand(a.setupAWSSSOCmd())
	return cmd
}

func (a *app) setupAWSSSOCmd() *cobra.Command {
	var session, role, region string
	cmd := &cobra.Command{
		Use:   "aws-sso --sso-session <name>",
		Short: "Generate ~/.aws/config profiles for every account in an AWS SSO session",
		Long: "Enumerate every account you can reach through an AWS IAM Identity Center\n" +
			"(SSO) session and write a `kr-<account>-<role>` profile for each into\n" +
			"~/.aws/config, so `kuberoutectl sync aws` (and plain aws/kubectl) can use\n" +
			"them. Requires an active SSO login first: `aws sso login --sso-session <name>`.\n" +
			"Existing profiles are never modified; only missing ones are appended.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if session == "" {
				return fmt.Errorf("--sso-session is required (the [sso-session <name>] block in ~/.aws/config)")
			}
			p, ok := a.registry.Get(aws.ProviderID)
			if !ok {
				return fmt.Errorf("aws provider is not registered")
			}
			awsProv, ok := p.(*aws.Provider)
			if !ok {
				return fmt.Errorf("aws provider does not support SSO population")
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			configPath := os.Getenv("AWS_CONFIG_FILE")
			if configPath == "" {
				configPath = filepath.Join(home, ".aws", "config")
			}
			cacheDir := filepath.Join(home, ".aws", "sso", "cache")

			res, err := awsProv.PopulateSSOProfiles(cmd.Context(), aws.SSOPopulateOptions{
				SessionName:   session,
				PreferredRole: role,
				Region:        region,
				ConfigPath:    configPath,
				CacheDir:      cacheDir,
				Progress:      stderrProgress{w: cmd.ErrOrStderr()},
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if a.output == formatJSON {
				return renderJSON(out, res)
			}
			fprintln(out, fmt.Sprintf("Across %d account(s): wrote %d profile(s), skipped %d already present.",
				res.Accounts, len(res.Written), len(res.Skipped)))
			for _, n := range res.Written {
				fprintln(out, "  +", n)
			}
			if len(res.Written) > 0 {
				fprintln(out, "Run `kuberoutectl sync aws` to discover clusters across these accounts.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "sso-session", "", "name of the [sso-session] block in ~/.aws/config")
	cmd.Flags().StringVar(&role, "role", "", "preferred role to select per account (default: AdministratorAccess if present, else first)")
	cmd.Flags().StringVar(&region, "region", "", "region set on generated profiles for EKS discovery (default: the session's sso_region)")
	return cmd
}
