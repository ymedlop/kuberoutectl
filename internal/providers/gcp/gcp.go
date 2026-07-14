// Package gcp is the GCP provider adapter. Like azure/aws it treats the
// `gcloud` CLI as a data source behind an injected CommandRunner, with all
// JSON->domain mapping in pure functions (parse.go, build.go).
//
// GCP's shape mirrors Azure more than AWS: a single active login spans many
// projects (as an Azure login spans subscriptions). So there is one source and
// one credential (the active gcloud account), projects are scopes, and GKE
// clusters are targets. The credential is OAuth-backed and renewable via
// `gcloud auth login`, so CanRenew is true and StaticCredentials is false.
package gcp

import (
	"context"
	"sort"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

const (
	// ProviderID is the stable identifier used in the registry and system labels.
	ProviderID domain.ProviderID = "gcp"
	// BinaryName is the CLI this provider drives.
	BinaryName = "gcloud"
)

// Provider implements providers.Provider for GCP.
type Provider struct {
	resolver execx.BinaryResolver
	runner   execx.CommandRunner
	now      func() time.Time
}

// New builds a GCP provider from a binary resolver and command runner.
func New(resolver execx.BinaryResolver, runner execx.CommandRunner) *Provider {
	return &Provider{
		resolver: resolver,
		runner:   runner,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// ID returns the provider identifier.
func (p *Provider) ID() domain.ProviderID { return ProviderID }

// Capabilities declares what GCP supports. The active login is renewable
// (`gcloud auth login`), so CanRenew is true and StaticCredentials is false.
func (p *Provider) Capabilities() domain.Capabilities {
	return domain.Capabilities{
		CanDiscoverScopes: true,
		CanRenew:          true,
		CanReauth:         true,
		CanSwitchContext:  true,
		StaticCredentials: false,
	}
}

// Discover reads the active gcloud login, enumerates projects as scopes, and
// lists GKE clusters per project as targets.
//
// Resilience mirrors the cloud providers: if there is no active login, a single
// expired credential is returned (hinting renew) with no scopes or targets and
// no error; a project whose cluster listing fails (e.g. GKE API disabled) is
// skipped rather than failing the whole sync.
func (p *Provider) Discover(ctx context.Context, in providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	prog := providers.ProgressOr(in.Progress)

	gcloud, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return providers.DiscoveryResult{}, err
	}
	now := p.now()

	prog.Step("reading gcloud config and account")
	var cfg gcpConfig
	if out, _, cErr := p.runner.Run(ctx, gcloud, "config", "list", "--format=json"); cErr == nil {
		cfg, _ = parseConfig(out)
	}
	var authAccounts []gcpAuthAccount
	if out, _, aErr := p.runner.Run(ctx, gcloud, "auth", "list", "--format=json"); aErr == nil {
		authAccounts, _ = parseAuthList(out)
	}

	account, authed := activeAccount(cfg.Core.Account, authAccounts)
	health, action := mapGCPHealth(authed)

	res := providers.DiscoveryResult{
		Sources:     []domain.AccessSource{buildSource(account, now)},
		Credentials: []domain.Credential{buildCredential(account, cfg.Core.Project, health, action, now)},
	}
	if !authed {
		prog.Step("no active gcloud account — run `gcloud auth login`")
		return res, nil
	}

	prog.Step("listing GCP projects (gcloud projects list)")
	projectsOut, _, err := p.runner.Run(ctx, gcloud, "projects", "list", "--format=json")
	if err != nil {
		// Authenticated but cannot list projects: surface the credential, no scopes.
		return res, nil
	}
	projects, err := parseProjects(projectsOut)
	if err != nil {
		return res, nil
	}
	prog.Step("found %d project(s)", len(projects))

	for i, proj := range projects {
		res.Scopes = append(res.Scopes, buildScope(proj))
		prog.Step("listing GKE clusters in %q (%d/%d)", proj.ProjectID, i+1, len(projects))
		res.Targets = append(res.Targets, p.discoverClusters(ctx, gcloud, account, proj, health, action, now)...)
	}

	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].ID < res.Targets[j].ID })
	prog.Step("discovered %d cluster(s)", len(res.Targets))
	return res, nil
}

// discoverClusters lists GKE clusters for one project. A failed or unparseable
// listing (commonly a project without the GKE API enabled) yields no targets
// rather than an error.
func (p *Provider) discoverClusters(ctx context.Context, gcloud, account string, proj gcpProject, health domain.AccessHealth, action domain.ActionHint, now time.Time) []domain.Target {
	out, _, err := p.runner.Run(ctx, gcloud, "container", "clusters", "list", "--project", proj.ProjectID, "--format=json")
	if err != nil {
		return nil
	}
	clusters, err := parseClusters(out)
	if err != nil {
		return nil
	}
	var targets []domain.Target
	for _, c := range clusters {
		targets = append(targets, buildTarget(account, proj, c, health, action, now))
	}
	return targets
}

// Renew re-authenticates the active gcloud login via `gcloud auth login`.
func (p *Provider) Renew(ctx context.Context, cred domain.Credential) error {
	return p.renew(ctx, cred)
}
