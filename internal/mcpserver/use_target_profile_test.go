package mcpserver

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache/jsonstore"
	"github.com/ymedlop/kuberoutectl/internal/services"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// credentialFakeProvider also implements providers.CredentialActivator, so the
// MCP path can be checked against a provider that honours a chosen credential.
type credentialFakeProvider struct {
	fakeProvider
	activatedAs *domain.Credential
}

func (f *credentialFakeProvider) ActivateAs(_ context.Context, t domain.Target, c domain.Credential) error {
	f.activated, f.activatedAs = &t, &c
	return nil
}

func multiCredentialSnapshot() domain.InventorySnapshot {
	return domain.InventorySnapshot{
		Credentials: []domain.Credential{
			{ID: "aws:ops", ProviderID: "aws", Name: "ops", Health: domain.HealthValid, Metadata: map[string]string{"profile": "ops"}},
			{ID: "aws:dev", ProviderID: "aws", Name: "dev", Health: domain.HealthExpired, Metadata: map[string]string{"profile": "dev"}},
		},
		Targets: []domain.Target{{
			ID: "aws:eks:prod", Name: "eks-prod", ProviderID: "aws", Platform: "eks",
			Health: domain.HealthValid, CredentialID: "aws:ops",
			CredentialIDs: []domain.CredentialID{"aws:ops", "aws:dev"},
		}},
	}
}

// newProfileHandler mirrors newTestHandler but keeps hold of the store, so a
// test can rewrite the snapshot to simulate a resync.
func newProfileHandler(t *testing.T) (*handler, *credentialFakeProvider, *jsonstore.Store) {
	t.Helper()
	prov := &credentialFakeProvider{fakeProvider: fakeProvider{
		id:   "aws",
		caps: domain.Capabilities{CanSwitchContext: true},
	}}
	dir := t.TempDir()
	store := jsonstore.New(dir, dir)
	if err := store.SaveSnapshot(multiCredentialSnapshot()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	reg := providers.NewRegistry()
	if err := reg.Register(prov); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	now := func() time.Time { return time.Unix(0, 0).UTC() }
	h := &handler{d: Deps{
		Version:   "test",
		Registry:  reg,
		Targets:   services.NewTargetService(store),
		Selection: services.NewSelectionService(store, reg, now),
	}}
	return h, prov, store
}

// An MCP client must be able to choose the access path, or it is stuck on the
// primary while the CLI is not — the same surface asymmetry that produced the
// v1.1.1 fix.
func TestMCPUseTarget_HonoursProfile(t *testing.T) {
	h, prov, _ := newProfileHandler(t)

	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"})
	if err != nil {
		t.Fatalf("useTarget: %v", err)
	}
	if out.Profile != "dev" {
		t.Errorf("Profile = %q, want dev", out.Profile)
	}
	if out.ProfileSource != "flag" {
		t.Errorf("ProfileSource = %q, want flag", out.ProfileSource)
	}
	if prov.activatedAs == nil || prov.activatedAs.ID != "aws:dev" {
		t.Errorf("ActivateAs got %+v, want aws:dev", prov.activatedAs)
	}
}

// The default must be reported as a default. A client that cannot tell a guess
// from a decision will present the primary as though it were chosen — and the
// primary is only the healthiest credential, not the one known to have access
// inside the cluster.
func TestMCPUseTarget_ReportsDefaultAsDefault(t *testing.T) {
	h, _, _ := newProfileHandler(t)

	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true})
	if err != nil {
		t.Fatalf("useTarget: %v", err)
	}
	if out.Profile != "ops" || out.ProfileSource != "default" {
		t.Errorf("Profile/ProfileSource = %q/%q, want ops/default", out.Profile, out.ProfileSource)
	}
}

// An unusable profile fails the tool call rather than silently activating
// something else.
func TestMCPUseTarget_RejectsUnreachableProfile(t *testing.T) {
	h, prov, _ := newProfileHandler(t)

	_, _, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "nope"})
	if err == nil {
		t.Fatal("expected an error for a profile that cannot reach this target")
	}
	if !strings.Contains(err.Error(), "ops") || !strings.Contains(err.Error(), "dev") {
		t.Errorf("error %q should list the profiles that would work", err)
	}
	if prov.activated != nil {
		t.Error("nothing must be activated when the choice is invalid")
	}
}

// CLI and MCP must converge: the same request through either surface leaves the
// same persisted selection. A divergence here is the class of bug that makes
// documentation say "the schema is the reliable source".
func TestMCPUseTarget_PersistsSameSelectionAsCLIPath(t *testing.T) {
	h, _, _ := newProfileHandler(t)

	if _, _, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"}); err != nil {
		t.Fatalf("useTarget: %v", err)
	}
	sel, err := h.d.Selection.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if sel.TargetID != "aws:eks:prod" {
		t.Errorf("TargetID = %q, want aws:eks:prod", sel.TargetID)
	}
	if sel.CredentialID != "aws:dev" {
		t.Errorf("CredentialID = %q, want aws:dev — the MCP path must persist the choice like the CLI does", sel.CredentialID)
	}
}

// The profile argument must be described in the tool schema, or a client has no
// way to discover it exists.
func TestMCPUseTargetInput_ProfileIsDocumented(t *testing.T) {
	rt := reflect.TypeOf(UseTargetInput{})
	for i := range rt.NumField() {
		f := rt.Field(i)
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "profile" {
			continue
		}
		if f.Tag.Get("jsonschema") == "" {
			t.Error("the profile field has no jsonschema description, so a client cannot discover it")
		}
		return
	}
	t.Fatal(`UseTargetInput has no field tagged json:"profile"`)
}

var _ providers.CredentialActivator = (*credentialFakeProvider)(nil)

// The MCP surface must report a lost profile too. This field had no test on
// either surface, which is exactly how a false positive shipped in it once.
func TestMCPUseTarget_ReportsALostProfile(t *testing.T) {
	h, _, store := newProfileHandler(t)

	if _, _, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"}); err != nil {
		t.Fatalf("first useTarget: %v", err)
	}
	// A resync drops dev from the cache, and the fold with it.
	snap := multiCredentialSnapshot()
	snap.Credentials = snap.Credentials[:1]
	snap.Targets[0].CredentialIDs = []domain.CredentialID{"aws:ops"}
	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("resync: %v", err)
	}

	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true})
	if err != nil {
		t.Fatalf("second useTarget: %v", err)
	}
	if out.LostProfile != "aws:dev" {
		t.Errorf("LostProfile = %q, want aws:dev", out.LostProfile)
	}
	if out.Profile != "ops" {
		t.Errorf("Profile = %q, want the fallback ops", out.Profile)
	}
}

// Switching deliberately between two live profiles is not a loss. Without the
// source gate this reported lost_profile on an ordinary workflow.
func TestMCPUseTarget_ExplicitSwitchIsNotALoss(t *testing.T) {
	h, _, _ := newProfileHandler(t)

	if _, _, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"}); err != nil {
		t.Fatalf("first useTarget: %v", err)
	}
	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "ops"})
	if err != nil {
		t.Fatalf("second useTarget: %v", err)
	}
	if out.LostProfile != "" {
		t.Errorf("LostProfile = %q, want empty: dev is still there, the client just chose ops", out.LostProfile)
	}
	if out.ProfileSource != "flag" {
		t.Errorf("ProfileSource = %q, want flag", out.ProfileSource)
	}
}
