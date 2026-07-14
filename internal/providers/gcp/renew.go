package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// renew re-authenticates the gcloud login via `gcloud auth login`. When the
// credential recorded an account, the login is scoped to it.
//
// Note: `gcloud auth login` is interactive (browser or `--no-launch-browser`
// device flow). It runs through the same CommandRunner as discovery for
// testability; a fully interactive terminal flow is a known limitation shared
// with the Azure provider.
func (p *Provider) renew(ctx context.Context, cred domain.Credential) error {
	gcloud, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	args := []string{"auth", "login"}
	if account := cred.Metadata["account"]; account != "" {
		args = append(args, account)
	}
	_, errOut, err := p.runner.Run(ctx, gcloud, args...)
	if err != nil {
		return fmt.Errorf("gcloud auth login failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
