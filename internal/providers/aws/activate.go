package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Activate runs `aws eks update-kubeconfig` to merge the cluster into the
// user's kubeconfig and set it as the current context. It uses the target's
// region and the profile recorded during discovery.
func (p *Provider) Activate(ctx context.Context, target domain.Target) error {
	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	if target.Region == "" {
		return fmt.Errorf("target %q is missing a region; run `kuberoutectl sync aws` again", target.ID)
	}
	args := []string{"eks", "update-kubeconfig", "--name", target.Name, "--region", target.Region}
	if profile := target.Metadata["profile"]; profile != "" {
		args = append(args, "--profile", profile)
	}
	_, errOut, err := p.runner.Run(ctx, awsBin, args...)
	if err != nil {
		return fmt.Errorf("aws eks update-kubeconfig failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
