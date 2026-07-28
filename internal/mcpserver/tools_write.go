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
	Profile  string `json:"profile,omitempty" jsonschema:"credential to go in through (an AWS profile name) when several reach the target; omit to use the target's default"`
}
type UseTargetOutput struct {
	Target    domain.Target `json:"target"`
	Activated bool          `json:"activated"`
	// Profile is the credential the target was activated through, and
	// ProfileSource says how it was picked — "flag", "remembered", or
	// "default". A client must be able to tell a choice from a guess: the
	// default is only the healthiest credential, which is not evidence that it
	// is the one with access inside the cluster.
	Profile       string `json:"profile,omitempty"`
	ProfileSource string `json:"profile_source,omitempty"`
	// LostCredentialID is the credential this target was previously used
	// through that the cache no longer offers. Set only when that happened, so a
	// client can tell "you are back on the default" from "you were always on
	// it".
	//
	// An id, not a name, unlike Profile above — deliberately. The credential is
	// gone from the snapshot, so there is no name left to resolve; naming the
	// field for a profile would promise something this value cannot be.
	LostCredentialID string `json:"lost_credential_id,omitempty"`
	// AccessWarning is set only when the chosen credential is *confirmed* to
	// hold no EKS access entry on this target — never when operability is merely
	// unknown, which is the common case. A field rather than prose so a client
	// can branch on it; the CLI prints the same string, from the same service, so
	// the two surfaces cannot drift into disagreeing about when to warn.
	AccessWarning string `json:"access_warning,omitempty"`
}

func (h *handler) useTarget(ctx context.Context, _ *mcp.CallToolRequest, in UseTargetInput) (*mcp.CallToolResult, UseTargetOutput, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	res, err := h.d.Selection.UseTarget(ctx, in.Ref, services.UseTargetOptions{
		Activate:       in.Activate,
		CredentialName: in.Profile,
	})
	if err != nil {
		return nil, UseTargetOutput{}, err
	}
	return nil, UseTargetOutput{
		Target:           res.Target,
		Activated:        in.Activate,
		Profile:          res.Credential.Name,
		ProfileSource:    res.CredentialSource.String(),
		LostCredentialID: string(res.LostCredentialID),
		AccessWarning:    res.AccessWarning,
	}, nil
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
