package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Renew re-authenticates the login behind a credential via `az login`. If the
// credential recorded a tenant, the login is scoped to it.
//
// Note: `az login` may be interactive (browser or device code). It runs
// through the same CommandRunner as discovery for testability; a fully
// interactive terminal flow is a known limitation tracked for a later slice.
func (p *Provider) Renew(ctx context.Context, cred domain.Credential) error {
	az, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	args := []string{"login"}
	if tenant := cred.Metadata["tenant_id"]; tenant != "" {
		args = append(args, "--tenant", tenant)
	}
	_, errOut, err := p.runner.Run(ctx, az, args...)
	if err != nil {
		return fmt.Errorf("az login failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
