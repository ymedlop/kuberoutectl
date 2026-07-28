package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// registerReadTools wires the side-effect-free tools. Each handler decodes its
// typed input, calls one service, and returns typed output — nothing more.
func (h *handler) registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "list_providers", Description: "List registered providers and their capabilities."}, h.listProviders)
	mcp.AddTool(s, &mcp.Tool{Name: "list_targets", Description: "List Kubernetes targets, optionally filtered by provider and/or label selector."}, h.listTargets)
	mcp.AddTool(s, &mcp.Tool{Name: "get_target", Description: "Look up a single target by reference (id, alias, or name)."}, h.getTarget)
	mcp.AddTool(s, &mcp.Tool{Name: "list_sources", Description: "List discovered access sources."}, h.listSources)
	mcp.AddTool(s, &mcp.Tool{Name: "list_scopes", Description: "List discovered scopes (e.g. subscriptions/accounts)."}, h.listScopes)
	mcp.AddTool(s, &mcp.Tool{Name: "list_credentials", Description: "List credentials with health and action hints (never any secret material), optionally filtered by provider."}, h.listCredentials)
	mcp.AddTool(s, &mcp.Tool{Name: "get_status", Description: "Show the current selection (target/collection) and how fresh the cache is."}, h.getStatus)
	mcp.AddTool(s, &mcp.Tool{Name: "list_collections", Description: "List saved collections."}, h.listCollections)
	mcp.AddTool(s, &mcp.Tool{Name: "get_collection", Description: "Resolve a collection to its current member targets."}, h.getCollection)
}

// ---- list_providers ----

// ProviderInfo is a provider's id plus its capability flags.
type ProviderInfo struct {
	ID           string              `json:"id"`
	Capabilities domain.Capabilities `json:"capabilities"`
}

type ListProvidersInput struct{}
type ListProvidersOutput struct {
	Providers []ProviderInfo `json:"providers"`
}

func (h *handler) listProviders(_ context.Context, _ *mcp.CallToolRequest, _ ListProvidersInput) (*mcp.CallToolResult, ListProvidersOutput, error) {
	out := ListProvidersOutput{Providers: []ProviderInfo{}}
	for _, p := range h.d.Registry.List() {
		out.Providers = append(out.Providers, ProviderInfo{ID: string(p.ID()), Capabilities: p.Capabilities()})
	}
	return nil, out, nil
}

// ---- list_targets ----

// The provider list in the jsonschema tag is hand-written (struct tags are
// compile-time constants) and is guarded by TestMCPSchemaListsEveryProvider in
// internal/cli — update it when a provider is added.
type ListTargetsInput struct {
	Provider      string `json:"provider,omitempty" jsonschema:"filter to one provider id (azure|aws|gcp|kubeconfig); empty for all"`
	Selector      string `json:"selector,omitempty" jsonschema:"label selector, e.g. env=prod or \"region in [eu-central-1]\""`
	IncludeHidden bool   `json:"include_hidden,omitempty" jsonschema:"include user-hidden targets (default false)"`
}
type ListTargetsOutput struct {
	Targets []domain.Target `json:"targets"`
}

func (h *handler) listTargets(_ context.Context, _ *mcp.CallToolRequest, in ListTargetsInput) (*mcp.CallToolResult, ListTargetsOutput, error) {
	f := services.TargetFilter{Provider: domain.ProviderID(in.Provider), IncludeHidden: in.IncludeHidden}
	if in.Selector != "" {
		sel, err := services.ParseSelector([]string{in.Selector})
		if err != nil {
			return nil, ListTargetsOutput{}, err
		}
		f.Selector = &sel
	}
	targets, err := h.d.Targets.List(f)
	if err != nil {
		return nil, ListTargetsOutput{}, err
	}
	return nil, ListTargetsOutput{Targets: targets}, nil
}

// ---- get_target ----

type GetTargetInput struct {
	Ref string `json:"ref" jsonschema:"target reference: full id, alias, or name"`
	// Refresh mirrors the CLI's `target inspect --refresh`: same name, same
	// default, same meaning, so a client and a human are never told different
	// things about the same cluster. Off by default because an agent may poll
	// this tool, and every other read tool here is a pure cache projection.
	Refresh bool `json:"refresh,omitempty" jsonschema:"re-check operability against the provider instead of using the last sync (default false)"`
}
type GetTargetOutput struct {
	Target domain.Target `json:"target"`
}

func (h *handler) getTarget(ctx context.Context, _ *mcp.CallToolRequest, in GetTargetInput) (*mcp.CallToolResult, GetTargetOutput, error) {
	// ResolveWithCredentials, not Resolve: the plain Resolve is shared with the
	// three `target label` commands, and teaching it to check access would give
	// them a cloud call none of them asked for.
	joined, err := services.TargetWithCredentials{}, error(nil)
	if in.Refresh {
		joined, err = h.d.Targets.ResolveWithAccessCheck(ctx, in.Ref)
	} else {
		// ResolveWithAccessCheck calls this itself, so the two are exclusive:
		// running both would load the snapshot twice for one request.
		joined, err = h.d.Targets.ResolveWithCredentials(in.Ref)
	}
	t := joined.Target
	if err != nil {
		return nil, GetTargetOutput{}, err
	}
	return nil, GetTargetOutput{Target: t}, nil
}

// ---- list_sources ----

type ListSourcesInput struct{}
type ListSourcesOutput struct {
	Sources []domain.AccessSource `json:"sources"`
}

func (h *handler) listSources(_ context.Context, _ *mcp.CallToolRequest, _ ListSourcesInput) (*mcp.CallToolResult, ListSourcesOutput, error) {
	src, err := h.d.Sources.List()
	if err != nil {
		return nil, ListSourcesOutput{}, err
	}
	return nil, ListSourcesOutput{Sources: src}, nil
}

// ---- list_scopes ----

type ListScopesInput struct{}
type ListScopesOutput struct {
	Scopes []domain.Scope `json:"scopes"`
}

func (h *handler) listScopes(_ context.Context, _ *mcp.CallToolRequest, _ ListScopesInput) (*mcp.CallToolResult, ListScopesOutput, error) {
	sc, err := h.d.Scopes.List()
	if err != nil {
		return nil, ListScopesOutput{}, err
	}
	return nil, ListScopesOutput{Scopes: sc}, nil
}

// ---- list_credentials ----

type ListCredentialsInput struct {
	Provider string `json:"provider,omitempty" jsonschema:"filter to one provider id; empty for all"`
}
type ListCredentialsOutput struct {
	// Credentials carry health/action and identifiers only — domain.Credential
	// holds no token/key material, so this never exposes a secret.
	Credentials []domain.Credential `json:"credentials"`
}

func (h *handler) listCredentials(_ context.Context, _ *mcp.CallToolRequest, in ListCredentialsInput) (*mcp.CallToolResult, ListCredentialsOutput, error) {
	creds, err := h.d.Credentials.List(domain.ProviderID(in.Provider))
	if err != nil {
		return nil, ListCredentialsOutput{}, err
	}
	return nil, ListCredentialsOutput{Credentials: creds}, nil
}

// ---- get_status ----

type GetStatusInput struct{}
type GetStatusOutput struct {
	Status services.SelectionStatus `json:"status"`
}

func (h *handler) getStatus(_ context.Context, _ *mcp.CallToolRequest, _ GetStatusInput) (*mcp.CallToolResult, GetStatusOutput, error) {
	st, err := h.d.Selection.Status()
	if err != nil {
		return nil, GetStatusOutput{}, err
	}
	return nil, GetStatusOutput{Status: st}, nil
}

// ---- list_collections ----

type ListCollectionsInput struct{}
type ListCollectionsOutput struct {
	Collections []domain.Collection `json:"collections"`
}

func (h *handler) listCollections(_ context.Context, _ *mcp.CallToolRequest, _ ListCollectionsInput) (*mcp.CallToolResult, ListCollectionsOutput, error) {
	cols, err := h.d.Collections.List()
	if err != nil {
		return nil, ListCollectionsOutput{}, err
	}
	return nil, ListCollectionsOutput{Collections: cols}, nil
}

// ---- get_collection ----

type GetCollectionInput struct {
	Name string `json:"name" jsonschema:"collection name"`
}
type GetCollectionOutput struct {
	Members []domain.Target `json:"members"`
}

func (h *handler) getCollection(_ context.Context, _ *mcp.CallToolRequest, in GetCollectionInput) (*mcp.CallToolResult, GetCollectionOutput, error) {
	members, err := h.d.Collections.Resolve(in.Name)
	if err != nil {
		return nil, GetCollectionOutput{}, err
	}
	return nil, GetCollectionOutput{Members: members}, nil
}
