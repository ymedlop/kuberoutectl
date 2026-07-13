package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Renew re-authenticates a profile according to its auth type. SSO and
// role-based profiles refresh via `aws sso login`. Static-key and unknown
// profiles are not renewable through the CLI, so Renew returns a clear manual
// instruction instead of pretending — this is the concrete behavior behind the
// provider's StaticCredentials capability.
func (p *Provider) Renew(ctx context.Context, cred domain.Credential) error {
	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	profile := cred.Metadata["profile"]
	if profile == "" {
		return fmt.Errorf("credential %q has no associated profile", cred.ID)
	}
	switch cred.Metadata["auth_type"] {
	case authSSO, authRole:
		_, errOut, err := p.runner.Run(ctx, awsBin, "sso", "login", "--profile", profile)
		if err != nil {
			return fmt.Errorf("aws sso login (profile %s) failed: %w: %s", profile, err, strings.TrimSpace(string(errOut)))
		}
		return nil
	default:
		return fmt.Errorf("profile %q uses non-renewable credentials; update ~/.aws/credentials or re-run `aws configure`", profile)
	}
}
