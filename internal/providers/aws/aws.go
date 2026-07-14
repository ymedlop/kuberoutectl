// Package aws is the AWS provider adapter. Like the azure package it treats the
// `aws` CLI as a data source behind an injected CommandRunner, with all
// JSON->domain mapping in pure functions (parse.go, build.go).
//
// AWS differs from Azure in two ways that shape this adapter:
//   - Access is per-profile: each profile is a distinct credential and may use
//     a different auth type (SSO, assumed-role, or static long-term keys).
//   - Not every credential is renewable. Static-key profiles are surfaced as
//     Health=static / Action=none|manual rather than being forced into a
//     cloud-session renewal model.
package aws

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

const (
	// ProviderID is the stable identifier used in the registry and system labels.
	ProviderID domain.ProviderID = "aws"
	// BinaryName is the CLI this provider drives.
	BinaryName = "aws"
)

// Provider implements providers.Provider for AWS.
type Provider struct {
	resolver execx.BinaryResolver
	runner   execx.CommandRunner
	now      func() time.Time
}

// New builds an AWS provider from a binary resolver and command runner.
func New(resolver execx.BinaryResolver, runner execx.CommandRunner) *Provider {
	return &Provider{
		resolver: resolver,
		runner:   runner,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// ID returns the provider identifier.
func (p *Provider) ID() domain.ProviderID { return ProviderID }

// Capabilities declares what AWS supports. CanRenew is true at the provider
// level (SSO/role profiles renew), but StaticCredentials is also true: some
// profiles are non-renewable, and that reality is expressed per-credential via
// Health/ActionHint rather than by lying about the provider.
func (p *Provider) Capabilities() domain.Capabilities {
	return domain.Capabilities{
		CanDiscoverScopes: true,
		CanRenew:          true,
		CanReauth:         true,
		CanSwitchContext:  true,
		StaticCredentials: true,
	}
}

// Discover enumerates profiles, validates each identity via STS, and lists EKS
// clusters in each profile's configured region.
//
// Resilience mirrors Azure: a profile whose STS check fails still yields a
// credential (with the right health/action) so the operator sees what needs
// attention; per-profile EKS reads that fail are skipped, not fatal.
func (p *Provider) Discover(ctx context.Context, in providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	prog := providers.ProgressOr(in.Progress)

	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return providers.DiscoveryResult{}, err
	}
	now := p.now()

	prog.Step("listing AWS profiles (aws configure list-profiles)")
	profilesOut, _, err := p.runner.Run(ctx, awsBin, "configure", "list-profiles")
	if err != nil {
		// No configured profiles is not an error — there is simply nothing to show.
		return providers.DiscoveryResult{}, nil
	}
	profiles := parseProfiles(profilesOut)
	sort.Strings(profiles)
	prog.Step("found %d profile(s)", len(profiles))

	res := providers.DiscoveryResult{}
	scopeSeen := map[domain.ScopeID]bool{}

	for i, profile := range profiles {
		res.Sources = append(res.Sources, buildSource(profile, now))

		prog.Step("validating identity for profile %q (%d/%d)", profile, i+1, len(profiles))
		identity, stsErr := p.callerIdentity(ctx, awsBin, profile)
		ssoURL := p.configGet(ctx, awsBin, profile, "sso_start_url")
		authType := classifyAuth(ssoURL, identity.Arn, stsErr == nil)
		health, action := mapAWSHealth(authType, stsErr == nil)

		res.Credentials = append(res.Credentials, buildCredential(profile, identity, authType, health, action, now))

		if stsErr != nil || identity.Account == "" {
			continue // unusable identity: no scope, no targets
		}
		if scope, ok := buildScope(identity.Account); ok && !scopeSeen[scope.ID] {
			scopeSeen[scope.ID] = true
			res.Scopes = append(res.Scopes, scope)
		}

		region := p.configGet(ctx, awsBin, profile, "region")
		if region == "" {
			continue // cannot list regional EKS without a region
		}
		prog.Step("listing EKS clusters for profile %q in %s", profile, region)
		res.Targets = append(res.Targets, p.discoverClusters(ctx, awsBin, profile, region, identity, health, action, now)...)
	}

	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].ID < res.Targets[j].ID })
	prog.Step("discovered %d cluster(s)", len(res.Targets))
	return res, nil
}

// discoverClusters lists and describes EKS clusters for one profile/region.
func (p *Provider) discoverClusters(ctx context.Context, awsBin, profile, region string, identity awsIdentity, health domain.AccessHealth, action domain.ActionHint, now time.Time) []domain.Target {
	listOut, _, err := p.runner.Run(ctx, awsBin, "eks", "list-clusters", "--profile", profile, "--region", region, "--output", "json")
	if err != nil {
		return nil
	}
	names, err := parseEKSList(listOut)
	if err != nil {
		return nil
	}
	var targets []domain.Target
	for _, name := range names {
		descOut, _, derr := p.runner.Run(ctx, awsBin, "eks", "describe-cluster", "--profile", profile, "--region", region, "--name", name, "--output", "json")
		if derr != nil {
			continue
		}
		cluster, perr := parseEKSDescribe(descOut)
		if perr != nil {
			continue
		}
		targets = append(targets, buildTarget(profile, region, identity, cluster, health, action, now))
	}
	return targets
}

// callerIdentity runs `aws sts get-caller-identity` for a profile.
func (p *Provider) callerIdentity(ctx context.Context, awsBin, profile string) (awsIdentity, error) {
	out, _, err := p.runner.Run(ctx, awsBin, "sts", "get-caller-identity", "--profile", profile, "--output", "json")
	if err != nil {
		return awsIdentity{}, err
	}
	return parseCallerIdentity(out)
}

// configGet reads a single profile config value. `aws configure get` exits
// non-zero when a key is unset, which we treat as an empty value.
func (p *Provider) configGet(ctx context.Context, awsBin, profile, key string) string {
	out, _, err := p.runner.Run(ctx, awsBin, "configure", "get", key, "--profile", profile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
