package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Activate runs `az aks get-credentials` to merge the cluster into the user's
// kubeconfig and set it as the current context. It reconstructs the required
// arguments from the target's scope (subscription) and its discovered metadata.
//
// `--overwrite-existing` avoids failures when a context entry for the cluster
// already exists; get-credentials sets the current context by default.
func (p *Provider) Activate(ctx context.Context, target domain.Target) error {
	az, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	rg := target.Metadata["resource_group"]
	if rg == "" {
		return fmt.Errorf("target %q is missing resource_group metadata; run `kuberoutectl sync azure` again", target.ID)
	}
	args := []string{
		"aks", "get-credentials",
		"--subscription", string(target.ScopeID),
		"--resource-group", rg,
		"--name", target.Name,
		"--overwrite-existing",
	}
	_, errOut, err := p.runner.Run(ctx, az, args...)
	if err != nil {
		return fmt.Errorf("az aks get-credentials failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
