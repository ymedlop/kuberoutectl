// Package azure is the Azure provider adapter. It treats the `az` CLI as a
// data source, not as the product: all external execution goes through an
// injected execx.CommandRunner, and all JSON->domain mapping lives in pure,
// unit-tested functions (parse.go, build.go). The core never imports this
// package — it is reached only through the providers.Provider interface.
package azure

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

const (
	// ProviderID is the stable identifier used in the registry and in
	// kuberoutectl.io/provider system labels.
	ProviderID domain.ProviderID = "azure"
	// BinaryName is the CLI this provider drives.
	BinaryName = "az"
	// sourceID is the single AccessSource representing the az login state.
	sourceID domain.SourceID = "azure-cli"
	// defaultExpiringWithin is how close to expiry a token is flagged
	// "expiring" rather than "valid". Azure access tokens live ~1h.
	defaultExpiringWithin = 5 * time.Minute
)

// Provider implements providers.Provider for Azure.
type Provider struct {
	resolver execx.BinaryResolver
	runner   execx.CommandRunner

	// now and expiringWithin are injectable so health mapping is deterministic
	// under test.
	now            func() time.Time
	expiringWithin time.Duration
}

// New builds an Azure provider from a binary resolver and command runner.
func New(resolver execx.BinaryResolver, runner execx.CommandRunner) *Provider {
	return &Provider{
		resolver:       resolver,
		runner:         runner,
		now:            func() time.Time { return time.Now().UTC() },
		expiringWithin: defaultExpiringWithin,
	}
}

// ID returns the provider identifier.
func (p *Provider) ID() domain.ProviderID { return ProviderID }

// Capabilities declares what Azure supports. Azure has a real subscription
// hierarchy and a renewable login, so scopes/renew/reauth are all true and it
// is not a static-credential provider.
func (p *Provider) Capabilities() domain.Capabilities {
	return domain.Capabilities{
		CanDiscoverScopes: true,
		CanRenew:          true,
		CanReauth:         true,
		CanSwitchContext:  false,
		StaticCredentials: false,
	}
}

// Discover reads Azure login state, subscriptions, and AKS clusters via `az`.
//
// It is intentionally resilient: if the user is not logged in (account list
// fails or is empty) it returns a single expired credential hinting renewal
// rather than an error, because "you need to log in" is useful operator
// information, not a tool failure. Per-subscription AKS reads that fail (e.g.
// missing permissions) are skipped, not fatal.
func (p *Provider) Discover(ctx context.Context, in providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	prog := providers.ProgressOr(in.Progress)

	az, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return providers.DiscoveryResult{}, err
	}
	now := p.now()

	prog.Step("querying Azure subscriptions (az account list)")
	accountsOut, accountsErr, err := p.runner.Run(ctx, az, "account", "list", "--output", "json")
	if err != nil {
		prog.Step("not logged in — run `az login`")
		return notLoggedInResult(now, string(accountsErr)), nil
	}
	accounts, err := parseAccounts(accountsOut)
	if err != nil {
		return providers.DiscoveryResult{}, fmt.Errorf("azure: parse account list: %w", err)
	}
	if len(accounts) == 0 {
		return notLoggedInResult(now, "az account list returned no subscriptions"), nil
	}
	// Deterministic subscription order drives both command order and output.
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	prog.Step("found %d subscription(s)", len(accounts))

	// Best-effort account-level health from the token cache. Azure keeps a
	// single token cache, so one probe approximates login health; multi-tenant
	// logins are an accepted MVP approximation (see build.go).
	prog.Step("checking credential health (az account get-access-token)")
	health, action := domain.HealthUnknown, domain.ActionRenew
	if tokenOut, _, tErr := p.runner.Run(ctx, az, "account", "get-access-token", "--output", "json"); tErr == nil {
		if tok, pErr := parseAccessToken(tokenOut); pErr == nil {
			health, action = mapHealth(tok, now, p.expiringWithin)
		}
	}

	res := providers.DiscoveryResult{
		Sources:     buildSources(now),
		Credentials: buildCredentials(accounts, health, action, now),
		Scopes:      buildScopes(accounts),
	}

	for i, acc := range accounts {
		if !strings.EqualFold(acc.State, "Enabled") {
			continue
		}
		prog.Step("listing AKS clusters in %q (%d/%d)", acc.Name, i+1, len(accounts))
		clustersOut, _, cErr := p.runner.Run(ctx, az, "aks", "list", "--subscription", acc.ID, "--output", "json")
		if cErr != nil {
			continue // unreadable subscription (permissions, disabled) — skip, not fatal
		}
		clusters, pErr := parseAKSClusters(clustersOut)
		if pErr != nil {
			continue
		}
		res.Targets = append(res.Targets, buildTargets(acc, clusters, health, action, now)...)
	}
	sort.Slice(res.Targets, func(i, j int) bool { return res.Targets[i].ID < res.Targets[j].ID })
	prog.Step("discovered %d cluster(s)", len(res.Targets))

	return res, nil
}
