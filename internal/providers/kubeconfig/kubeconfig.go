// Package kubeconfig is the kubeconfig provider adapter. It surfaces the
// clusters, users, and contexts already present in the user's kubeconfig as
// first-class inventory, so self-hosted and local clusters sit alongside the
// cloud providers.
//
// Unlike azure/aws it does not authenticate anything: a kubeconfig is a static
// artifact. That shapes the adapter:
//
//   - The `kubectl` CLI is the data source (`kubectl config view --raw -o json`),
//     driven through the same injected CommandRunner, with all JSON->domain
//     mapping in pure functions (parse.go, build.go). No new dependency, and
//     parsing stays testable against fixtures.
//   - Credentials are not renewable by kuberoutectl. A client cert or bearer
//     token is Health=static / Action=none; an exec / auth-provider credential
//     is externally managed and reported Health=unknown. This is the concrete
//     realization of "some credentials are static and not renewable".
//   - Switching context is cheap: the context already exists, so activation is
//     `kubectl config use-context`, not a credential fetch.
package kubeconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

const (
	// ProviderID is the stable identifier used in the registry and system labels.
	ProviderID domain.ProviderID = "kubeconfig"
	// BinaryName is the CLI this provider drives.
	BinaryName = "kubectl"
)

// Provider implements providers.Provider for kubeconfig.
type Provider struct {
	resolver execx.BinaryResolver
	runner   execx.CommandRunner
	now      func() time.Time
}

// New builds a kubeconfig provider from a binary resolver and command runner.
func New(resolver execx.BinaryResolver, runner execx.CommandRunner) *Provider {
	return &Provider{
		resolver: resolver,
		runner:   runner,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// ID returns the provider identifier.
func (p *Provider) ID() domain.ProviderID { return ProviderID }

// Capabilities declares what kubeconfig supports. It can discover its scopes
// (clusters) and switch context, but it cannot renew: a kubeconfig credential
// is static or externally managed, so CanRenew is false and StaticCredentials
// is true.
func (p *Provider) Capabilities() domain.Capabilities {
	return domain.Capabilities{
		CanDiscoverScopes: true,
		CanRenew:          false,
		CanReauth:         false,
		CanSwitchContext:  true,
		StaticCredentials: true,
		OverlayProvider:   true,
	}
}

// Discover reads the merged kubeconfig and maps clusters->scopes,
// users->credentials, contexts->targets. An empty kubeconfig is not an error;
// it simply yields nothing.
func (p *Provider) Discover(ctx context.Context, in providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	prog := providers.ProgressOr(in.Progress)

	kubectl, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return providers.DiscoveryResult{}, err
	}
	now := p.now()

	prog.Step("reading kubeconfig (kubectl config view)")
	out, _, err := p.runner.Run(ctx, kubectl, "config", "view", "--raw", "-o", "json")
	if err != nil {
		return providers.DiscoveryResult{}, fmt.Errorf("kubectl config view: %w", err)
	}
	cfg, err := parseConfig(out)
	if err != nil {
		return providers.DiscoveryResult{}, err
	}
	if len(cfg.Contexts) == 0 {
		prog.Step("no contexts in kubeconfig")
		return providers.DiscoveryResult{}, nil
	}

	res := providers.DiscoveryResult{Sources: []domain.AccessSource{buildSource(kubeconfigLocation(), now)}}
	prog.Step("found %d cluster(s), %d user(s), %d context(s)", len(cfg.Clusters), len(cfg.Users), len(cfg.Contexts))

	serverByCluster := map[string]string{}
	for _, c := range cfg.Clusters {
		serverByCluster[c.Name] = c.Cluster.Server
		res.Scopes = append(res.Scopes, buildScope(c))
	}

	healthByUser := map[string]credHealth{}
	for _, u := range cfg.Users {
		authType := classifyUserAuth(u.User)
		health, action := mapKubeconfigHealth(authType)
		healthByUser[u.Name] = credHealth{health: health, action: action, authType: authType}
		res.Credentials = append(res.Credentials, buildCredential(u.Name, authType, health, action, now))
	}

	for _, cx := range cfg.Contexts {
		// A context may reference a user that has no users[] entry (rare, but
		// valid); treat that as an unknown, do-nothing credential rather than
		// dropping the target.
		h, ok := healthByUser[cx.Context.User]
		if !ok {
			h = credHealth{health: domain.HealthUnknown, action: domain.ActionNone, authType: authUnknown}
		}
		res.Targets = append(res.Targets, buildTarget(cx, serverByCluster[cx.Context.Cluster], h.health, h.action, cx.Name == cfg.CurrentContext, now))
	}

	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].ID < res.Targets[j].ID })
	prog.Step("discovered %d context(s)", len(res.Targets))
	return res, nil
}

// Renew is unsupported: kubeconfig credentials are static or externally
// managed. Capabilities reports CanRenew=false, so services never call this,
// but it returns a clear error if invoked directly.
func (p *Provider) Renew(context.Context, domain.Credential) error {
	return providers.ErrUnsupported
}

// credHealth bundles a user's derived health so contexts can inherit it.
type credHealth struct {
	health   domain.AccessHealth
	action   domain.ActionHint
	authType string
}

// kubeconfigLocation reports where the active kubeconfig lives, for display as
// the source location. It honors KUBECONFIG and falls back to ~/.kube/config.
func kubeconfigLocation() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "config")
	}
	return "~/.kube/config"
}
