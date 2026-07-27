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
	return p.updateKubeconfig(ctx, target, target.Metadata["profile"])
}

// ActivateAs implements providers.CredentialActivator: it activates through the
// given credential instead of the target's recorded primary, which is what
// makes `target use <ref> --profile <name>` mean anything for a cluster several
// profiles can reach.
//
// The credential must carry a profile. Erroring beats falling back to the
// primary: a silent fallback would put the operator on a different identity
// than the one they explicitly asked for, and in a fleet where profiles differ
// in privilege that is exactly the mistake worth failing loudly on.
func (p *Provider) ActivateAs(ctx context.Context, target domain.Target, cred domain.Credential) error {
	profile := cred.Metadata["profile"]
	if profile == "" {
		return fmt.Errorf("credential %q carries no AWS profile; re-run `kuberoutectl sync aws`", cred.Name)
	}
	return p.updateKubeconfig(ctx, target, profile)
}

// updateKubeconfig runs `aws eks update-kubeconfig` for one target/profile pair.
// An empty profile omits the flag, letting the aws CLI apply its own default —
// the behaviour Activate has always had for targets discovered without one.
func (p *Provider) updateKubeconfig(ctx context.Context, target domain.Target, profile string) error {
	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	if target.Region == "" {
		return fmt.Errorf("target %q is missing a region; run `kuberoutectl sync aws` again", target.ID)
	}
	args := []string{"eks", "update-kubeconfig", "--name", target.Name, "--region", target.Region}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	_, errOut, err := p.runner.Run(ctx, awsBin, args...)
	if err != nil {
		return fmt.Errorf("aws eks update-kubeconfig failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
