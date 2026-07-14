package kubeconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Activate makes the target's context current via `kubectl config use-context`.
// The context already exists in the kubeconfig, so unlike the cloud providers
// there is no credential fetch — just a current-context switch. The context
// name is the target's Name.
func (p *Provider) Activate(ctx context.Context, target domain.Target) error {
	kubectl, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	_, errOut, err := p.runner.Run(ctx, kubectl, "config", "use-context", target.Name)
	if err != nil {
		return fmt.Errorf("kubectl config use-context %q failed: %w: %s", target.Name, err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
