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
	// Each profile's identity, reduced to the form an access entry can be
	// compared against. Collected here because the access-entry check runs after
	// the per-profile loop, by which point the STS responses are gone.
	principalKeys := map[domain.CredentialID]string{}

	for i, profile := range profiles {
		res.Sources = append(res.Sources, buildSource(profile, now))

		prog.Step("validating identity for profile %q (%d/%d)", profile, i+1, len(profiles))
		identity, stsErr := p.callerIdentity(ctx, awsBin, profile)
		// SSO is signalled either by sso_start_url directly on the profile (legacy
		// format) or by an sso_session reference (modern format, where the start
		// URL lives under a separate [sso-session] section). Read both so an
		// expired sso_session profile isn't misclassified as unknown.
		ssoURL := p.configGet(ctx, awsBin, profile, "sso_start_url")
		ssoSession := p.configGet(ctx, awsBin, profile, "sso_session")
		authType := classifyAuth(ssoURL, ssoSession, identity.Arn, stsErr == nil)
		health, action := mapAWSHealth(authType, stsErr == nil)

		res.Credentials = append(res.Credentials, buildCredential(profile, identity, authType, health, action, now))

		if stsErr != nil {
			// Surface why this profile yields nothing — most often an expired
			// token — instead of leaving the operator to guess from empty counts.
			prog.Step("%s", authFailureHint(profile, authType))
		}
		if stsErr != nil || identity.Account == "" {
			continue // unusable identity: no scope, no targets
		}
		principalKeys[credentialID(profile)] = principalKey(identity.Arn)
		if scope, ok := buildScope(identity.Account); ok && !scopeSeen[scope.ID] {
			scopeSeen[scope.ID] = true
			res.Scopes = append(res.Scopes, scope)
		}

		region := p.configGet(ctx, awsBin, profile, "region")
		if region == "" {
			continue // cannot list regional EKS without a region
		}
		prog.Step("listing EKS clusters for profile %q in %s", profile, region)
		res.Targets = append(res.Targets, p.discoverClusters(ctx, awsBin, profile, region, identity, health, action, now, prog)...)
	}

	// Several profiles into one account see the same cluster, and an EKS ARN is
	// account-scoped — so the per-profile targets above can share an ID. Group
	// them, ask which profiles the cluster actually admits, then fold each group
	// into one target.
	//
	// The check sits between the two halves on purpose: its answer feeds the
	// ranking, which the fold performs. Folding first and re-ranking afterwards
	// is impossible — the fold keeps the winner's own struct and discards the
	// losing candidates'.
	//
	// Every cluster is checked, not only those several profiles reach. The
	// original bound reasoned that with one way in there is nothing to choose,
	// which is true — but there is still something to know: whether that one way
	// in will be refused. A fleet where each cluster has a single profile got no
	// verdict at all under the old rule, which is most of the value of having the
	// check. `CONFIG_MAP` clusters still cost nothing, since the mode comes from
	// the describe response already in hand.
	groups := groupTargetsByID(res.Targets)
	folded := make([]domain.Target, 0, len(groups))
	checked := 0
	for _, group := range groups {
		access, cErr := p.checkAccessEntries(ctx, awsBin, group, principalKeys, prog)
		if cErr != nil {
			return providers.DiscoveryResult{}, cErr
		}
		// Counting the clusters an entry list was actually fetched for, not the
		// ones a verdict was reached about: a CONFIG_MAP cluster yields a verdict
		// from the describe response already in hand and costs nothing, so
		// including it would overstate what the sync spent.
		switch access.check {
		case domain.AccessCheckAPI, domain.AccessCheckAPIAndConfigMap, domain.AccessCheckUnavailable:
			checked++
		}
		folded = append(folded, foldGroup(group, access))
	}
	res.Targets = folded

	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].ID < res.Targets[j].ID })
	prog.Step("discovered %d cluster(s); listed access entries for %d", len(res.Targets), checked)
	return res, nil
}

// discoverClusters lists and describes EKS clusters for one profile/region.
//
// The two calls fail for different reasons, and the difference is the whole
// reason reachability is knowable: eks:ListClusters takes no cluster in its
// request, so IAM cannot scope it below account/region, while
// eks:DescribeCluster names the cluster in its resource ARN and IAM does scope
// it per cluster. So a profile that lists everything may still be denied on
// individual clusters — which is exactly how a fleet with no documented access
// pattern gets mapped. Each denial is reported through prog rather than
// silently skipped: that step output is the access map.
func (p *Provider) discoverClusters(ctx context.Context, awsBin, profile, region string, identity awsIdentity, health domain.AccessHealth, action domain.ActionHint, now time.Time, prog providers.Progress) []domain.Target {
	listOut, _, err := p.runner.Run(ctx, awsBin, "eks", "list-clusters", "--profile", profile, "--region", region, "--output", "json")
	if err != nil {
		return nil
	}
	names, err := parseEKSList(listOut)
	if err != nil {
		// Resilient like the command failure above — one profile's unreadable
		// listing must not sink the whole sync — but never silent. The command
		// succeeded, so `--verbose` shows nothing wrong either, and without this
		// an aws CLI output-format change reads as "you have no clusters".
		prog.Step("could not parse the cluster list for profile %q in %s (%v) — possible aws CLI format change; skipping this profile", profile, region, err)
		return nil
	}
	var targets []domain.Target
	for _, name := range names {
		descOut, _, derr := p.runner.Run(ctx, awsBin, "eks", "describe-cluster", "--profile", profile, "--region", region, "--name", name, "--output", "json")
		if derr != nil {
			prog.Step("profile %q cannot describe cluster %q in %s — skipping it for this profile", profile, name, region)
			continue
		}
		cluster, perr := parseEKSDescribe(descOut)
		if perr != nil {
			// Deliberately worded differently from the access denial above: that
			// one is routine in a fleet with uneven permissions, this one is a
			// format regression worth investigating. Same wording for both would
			// bury the rare case in the common one.
			prog.Step("could not parse the description of cluster %q for profile %q (%v) — possible aws CLI format change; skipping this cluster", name, profile, perr)
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
