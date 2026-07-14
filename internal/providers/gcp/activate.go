package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Activate runs `gcloud container clusters get-credentials` to merge the GKE
// cluster into the user's kubeconfig and set it as the current context. GKE
// clusters are regional or zonal; `--location` accepts either, and is taken
// together with the project from the target's discovered metadata.
func (p *Provider) Activate(ctx context.Context, target domain.Target) error {
	gcloud, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return err
	}
	location := target.Metadata["location"]
	if location == "" {
		location = target.Region
	}
	if location == "" {
		return fmt.Errorf("target %q is missing a location; run `kuberoutectl sync gcp` again", target.ID)
	}
	args := []string{"container", "clusters", "get-credentials", target.Name, "--location", location}
	if project := target.Metadata["project"]; project != "" {
		args = append(args, "--project", project)
	}
	_, errOut, err := p.runner.Run(ctx, gcloud, args...)
	if err != nil {
		return fmt.Errorf("gcloud container clusters get-credentials failed: %w: %s", err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
