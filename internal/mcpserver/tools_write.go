package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/services"
)

// registerWriteTools wires the safe, non-destructive mutating tools. Each holds
// h.mu for its whole load→mutate→save so overlapping calls can't lose an
// update. No destructive operation (delete/clear/renew/label/hide) is exposed.
func (h *handler) registerWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "sync_provider", Description: "Discover inventory from a provider and update the local cache."}, h.syncProvider)
	mcp.AddTool(s, &mcp.Tool{Name: "use_target", Description: "Select a target as current; with activate, also merge its context into the local kubeconfig."}, h.useTarget)
	mcp.AddTool(s, &mcp.Tool{Name: "create_or_update_collection", Description: "Create a collection, or update it in place if the name already exists."}, h.createOrUpdateCollection)
}

// ---- sync_provider ----

// The provider list in the jsonschema tag is hand-written (struct tags are
// compile-time constants) and is guarded by TestMCPSchemaListsEveryProvider in
// internal/cli — update it when a provider is added.
type SyncProviderInput struct {
	Provider string `json:"provider" jsonschema:"provider id to sync (azure|aws|gcp|kubeconfig)"`
}

// SyncProviderOutput mirrors the CLI `sync` summary counts.
type SyncProviderOutput struct {
	Provider    string `json:"provider"`
	Sources     int    `json:"sources"`
	Credentials int    `json:"credentials"`
	Scopes      int    `json:"scopes"`
	Targets     int    `json:"targets"`
}

func (h *handler) syncProvider(ctx context.Context, _ *mcp.CallToolRequest, in SyncProviderInput) (*mcp.CallToolResult, SyncProviderOutput, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := domain.ProviderID(in.Provider)
	snap, err := h.d.Discovery.Sync(ctx, id, providers.NopProgress{})
	if err != nil {
		return nil, SyncProviderOutput{}, err
	}
	out := SyncProviderOutput{Provider: in.Provider}
	for _, s := range snap.Sources {
		if s.ProviderID == id {
			out.Sources++
		}
	}
	for _, c := range snap.Credentials {
		if c.ProviderID == id {
			out.Credentials++
		}
	}
	for _, s := range snap.Scopes {
		if s.ProviderID == id {
			out.Scopes++
		}
	}
	for _, t := range snap.Targets {
		if t.ProviderID == id {
			out.Targets++
		}
	}
	return nil, out, nil
}

// ---- use_target ----

type UseTargetInput struct {
	Ref      string `json:"ref" jsonschema:"target reference: full id, alias, or name"`
	Activate bool   `json:"activate,omitempty" jsonschema:"also merge the target's context into ~/.kube/config and make it current (default false)"`
}
type UseTargetOutput struct {
	Target    domain.Target `json:"target"`
	Activated bool          `json:"activated"`
}

func (h *handler) useTarget(ctx context.Context, _ *mcp.CallToolRequest, in UseTargetInput) (*mcp.CallToolResult, UseTargetOutput, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	t, err := h.d.Selection.UseTarget(ctx, in.Ref, in.Activate)
	if err != nil {
		return nil, UseTargetOutput{}, err
	}
	return nil, UseTargetOutput{Target: t, Activated: in.Activate}, nil
}

// ---- create_or_update_collection ----

type CreateOrUpdateCollectionInput struct {
	Name        string   `json:"name" jsonschema:"collection name (also its id)"`
	Description string   `json:"description,omitempty" jsonschema:"optional human description"`
	Selector    string   `json:"selector,omitempty" jsonschema:"label selector defining membership, e.g. env=prod"`
	StaticIDs   []string `json:"static_ids,omitempty" jsonschema:"explicit target ids to include in addition to selector matches"`
}
type CreateOrUpdateCollectionOutput struct {
	Collection domain.Collection `json:"collection"`
}

func (h *handler) createOrUpdateCollection(_ context.Context, _ *mcp.CallToolRequest, in CreateOrUpdateCollectionInput) (*mcp.CallToolResult, CreateOrUpdateCollectionOutput, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var sel domain.LabelSelector
	if in.Selector != "" {
		parsed, err := services.ParseSelector([]string{in.Selector})
		if err != nil {
			return nil, CreateOrUpdateCollectionOutput{}, err
		}
		sel = parsed
	}
	static := make([]domain.TargetID, 0, len(in.StaticIDs))
	for _, id := range in.StaticIDs {
		static = append(static, domain.TargetID(id))
	}
	col, err := h.d.Collections.Save(in.Name, in.Description, sel, static)
	if err != nil {
		return nil, CreateOrUpdateCollectionOutput{}, err
	}
	return nil, CreateOrUpdateCollectionOutput{Collection: col}, nil
}
