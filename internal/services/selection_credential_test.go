package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// credentialActivatableProvider implements ContextActivator AND
// CredentialActivator, recording which credential activation went through.
type credentialActivatableProvider struct {
	activatableProvider
	activatedAs *domain.Credential
}

func (c *credentialActivatableProvider) ActivateAs(_ context.Context, t domain.Target, cred domain.Credential) error {
	if c.activateErr != nil {
		return c.activateErr
	}
	tt, cc := t, cred
	c.activated, c.activatedAs = &tt, &cc
	return nil
}

// storeWithMultiCredentialTarget models one cluster reachable two ways: `ops`
// is the primary, `dev` the alternative.
func storeWithMultiCredentialTarget() *memStore {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{
		Credentials: []domain.Credential{
			{ID: "aws:ops", ProviderID: "aws", Name: "ops", Health: domain.HealthValid, Metadata: map[string]string{"profile": "ops"}},
			{ID: "aws:dev", ProviderID: "aws", Name: "dev", Health: domain.HealthExpired, Metadata: map[string]string{"profile": "dev"}},
		},
		Targets: []domain.Target{{
			ID: "t1", ProviderID: "aws", Name: "eks-prod",
			CredentialID:  "aws:ops",
			CredentialIDs: []domain.CredentialID{"aws:ops", "aws:dev"},
		}},
	}
	return m
}

func multiCredentialSetup() (*memStore, *credentialActivatableProvider, *SelectionService) {
	store := storeWithMultiCredentialTarget()
	prov := &credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)
	return store, prov, NewSelectionService(store, reg, fixedNow)
}

// With no choice made, activation goes through the primary and the result says
// the choice was a default — the CLI needs that to avoid presenting a guess as
// a decision.
func TestUseTarget_DefaultsToPrimaryAndSaysSo(t *testing.T) {
	store, prov, svc := multiCredentialSetup()

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.Credential.ID != "aws:ops" {
		t.Errorf("activated credential = %q, want the primary aws:ops", res.Credential.ID)
	}
	if res.CredentialSource != CredentialFromDefault {
		t.Errorf("CredentialSource = %v, want CredentialFromDefault", res.CredentialSource)
	}
	if prov.activatedAs == nil || prov.activatedAs.ID != "aws:ops" {
		t.Errorf("ActivateAs got %+v, want aws:ops", prov.activatedAs)
	}
	if store.selection.CredentialID != "aws:ops" {
		t.Errorf("persisted credential = %q, want aws:ops", store.selection.CredentialID)
	}
}

// An explicit name wins and is reported as an explicit choice.
func TestUseTarget_ExplicitCredentialNameWins(t *testing.T) {
	store, prov, svc := multiCredentialSetup()

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.Credential.ID != "aws:dev" {
		t.Errorf("activated credential = %q, want aws:dev", res.Credential.ID)
	}
	if res.CredentialSource != CredentialFromFlag {
		t.Errorf("CredentialSource = %v, want CredentialFromFlag", res.CredentialSource)
	}
	if prov.activatedAs == nil || prov.activatedAs.Metadata["profile"] != "dev" {
		t.Errorf("ActivateAs got %+v, want the dev profile", prov.activatedAs)
	}
	if store.selection.CredentialID != "aws:dev" {
		t.Errorf("persisted credential = %q, want aws:dev", store.selection.CredentialID)
	}
}

// Persistence closes the loop: a later bare `target use` reuses the choice
// rather than reverting to the primary, and reports that it was remembered.
func TestUseTarget_RemembersPreviousChoice(t *testing.T) {
	_, prov, svc := multiCredentialSetup()

	if _, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"}); err != nil {
		t.Fatalf("first UseTarget: %v", err)
	}
	prov.activatedAs = nil

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("second UseTarget: %v", err)
	}
	if res.Credential.ID != "aws:dev" {
		t.Errorf("credential = %q, want the remembered aws:dev", res.Credential.ID)
	}
	if res.CredentialSource != CredentialFromMemory {
		t.Errorf("CredentialSource = %v, want CredentialFromMemory", res.CredentialSource)
	}
	if prov.activatedAs == nil || prov.activatedAs.ID != "aws:dev" {
		t.Errorf("ActivateAs got %+v, want aws:dev", prov.activatedAs)
	}
}

// Edge case 5: a remembered credential a resync dropped must not strand the
// operator. Fall back to the primary rather than failing on a phantom profile.
func TestUseTarget_RemembersNothingWhenCredentialVanished(t *testing.T) {
	store, prov, svc := multiCredentialSetup()
	store.selection = domain.Selection{TargetID: "t1", CredentialID: "aws:gone"}

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.Credential.ID != "aws:ops" {
		t.Errorf("credential = %q, want a fallback to the primary aws:ops", res.Credential.ID)
	}
	if res.CredentialSource != CredentialFromDefault {
		t.Errorf("CredentialSource = %v, want CredentialFromDefault", res.CredentialSource)
	}
	if prov.activatedAs == nil {
		t.Error("activation should still have happened")
	}
}

// Edge case 6: an unusable name is rejected before any external CLI runs, and
// the error names the profiles that would have worked.
func TestUseTarget_UnknownCredentialNameRejectedBeforeActivating(t *testing.T) {
	_, prov, svc := multiCredentialSetup()

	_, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "nope"})
	if err == nil {
		t.Fatal("expected an error for a credential that cannot reach this target")
	}
	for _, want := range []string{"nope", "ops", "dev"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if prov.activated != nil {
		t.Error("nothing must be activated when the choice is invalid")
	}
}

// Edge case 7: the same rejection for a target with exactly one way in — a
// silent no-op would leave the operator believing a choice took effect.
func TestUseTarget_ProfileOnSingleCredentialTargetRejected(t *testing.T) {
	store := newMemStore()
	store.snap = domain.InventorySnapshot{
		Credentials: []domain.Credential{{ID: "gcp:account:me", ProviderID: "gcp", Name: "me"}},
		Targets:     []domain.Target{{ID: "t1", ProviderID: "gcp", Name: "gke-prod", CredentialID: "gcp:account:me"}},
	}
	prov := &credentialActivatableProvider{activatableProvider: activatableProvider{id: "gcp", canSwitch: true}}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)
	svc := NewSelectionService(store, reg, fixedNow)

	_, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "other"})
	if err == nil {
		t.Fatal("expected an error: this target has only one credential")
	}
	if prov.activated != nil {
		t.Error("nothing must be activated when the choice is invalid")
	}
}

// A provider that cannot activate through a named credential must reject an
// explicit choice rather than quietly using the primary, which would put the
// operator on an identity they did not pick.
func TestUseTarget_ExplicitChoiceRejectedWhenProviderCannotHonourIt(t *testing.T) {
	store := storeWithMultiCredentialTarget()
	plain := &activatableProvider{id: "aws", canSwitch: true} // ContextActivator only
	reg := providers.NewRegistry()
	_ = reg.Register(plain)
	svc := NewSelectionService(store, reg, fixedNow)

	_, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"})
	if err == nil {
		t.Fatal("expected an error: this provider cannot activate through a chosen credential")
	}
	if plain.activated != nil {
		t.Error("nothing must be activated when the choice cannot be honoured")
	}
}

// Recording a selection without touching the kubeconfig still resolves and
// persists the credential, so `current` stays truthful either way.
func TestUseTarget_NoKubeconfigStillRecordsCredential(t *testing.T) {
	store, prov, svc := multiCredentialSetup()

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{CredentialName: "dev"})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if prov.activated != nil {
		t.Error("activation must not run with Activate=false")
	}
	if res.Credential.ID != "aws:dev" || store.selection.CredentialID != "aws:dev" {
		t.Errorf("credential = %q / persisted %q, want aws:dev both", res.Credential.ID, store.selection.CredentialID)
	}
}

// Status reports the credential the selection was activated through, so
// `current` can answer "how am I actually getting in".
func TestStatus_ReportsSelectedCredential(t *testing.T) {
	_, _, svc := multiCredentialSetup()
	if _, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"}); err != nil {
		t.Fatalf("UseTarget: %v", err)
	}

	st, err := svc.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Credential == nil {
		t.Fatal("Status.Credential is nil, want the selected credential")
	}
	if st.Credential.Name != "dev" {
		t.Errorf("Credential.Name = %q, want dev", st.Credential.Name)
	}
	if st.CredentialMissing {
		t.Error("CredentialMissing must be false while the credential is in the cache")
	}
}

// Edge case 5, the `current` half: a selection naming a credential a resync
// dropped is flagged rather than silently rendered as if nothing changed.
func TestStatus_FlagsVanishedCredential(t *testing.T) {
	store, _, svc := multiCredentialSetup()
	store.selection = domain.Selection{TargetID: "t1", CredentialID: "aws:gone"}

	st, err := svc.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Credential != nil {
		t.Errorf("Credential = %+v, want nil for a credential no longer cached", st.Credential)
	}
	if !st.CredentialMissing {
		t.Error("CredentialMissing must be true so `current` can say the profile is gone")
	}
}

// A deliberate switch between two live profiles is not a loss. The chosen
// credential differs from the remembered one by definition, so a check on the
// difference alone raises a false alarm on an ordinary workflow.
func TestUseTarget_ExplicitSwitchIsNotReportedAsALoss(t *testing.T) {
	_, _, svc := multiCredentialSetup()

	if _, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"}); err != nil {
		t.Fatalf("first UseTarget: %v", err)
	}
	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "ops"})
	if err != nil {
		t.Fatalf("second UseTarget: %v", err)
	}
	if res.LostCredentialID != "" {
		t.Errorf("LostCredentialID = %q, want empty: dev is still present, the operator simply chose ops",
			res.LostCredentialID)
	}
	if res.CredentialSource != CredentialFromFlag {
		t.Errorf("CredentialSource = %v, want CredentialFromFlag", res.CredentialSource)
	}
}

// The genuine case: the remembered credential is gone from the snapshot, so the
// fallback to the default must be attributable.
func TestUseTarget_VanishedRememberedCredentialIsReportedAsALoss(t *testing.T) {
	store, _, svc := multiCredentialSetup()

	if _, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"}); err != nil {
		t.Fatalf("first UseTarget: %v", err)
	}
	// A resync drops dev from ~/.aws/config, and the fold with it.
	store.snap.Credentials = store.snap.Credentials[:1]
	store.snap.Targets[0].CredentialIDs = []domain.CredentialID{"aws:ops"}

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("second UseTarget: %v", err)
	}
	if res.LostCredentialID != "aws:dev" {
		t.Errorf("LostCredentialID = %q, want aws:dev", res.LostCredentialID)
	}
	if res.Credential.ID != "aws:ops" {
		t.Errorf("credential = %q, want a fallback to aws:ops", res.Credential.ID)
	}
}

// Reusing a remembered credential that is still there is not a loss either.
func TestUseTarget_RememberedAndStillPresentIsNotALoss(t *testing.T) {
	_, _, svc := multiCredentialSetup()

	if _, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"}); err != nil {
		t.Fatalf("first UseTarget: %v", err)
	}
	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("second UseTarget: %v", err)
	}
	if res.LostCredentialID != "" {
		t.Errorf("LostCredentialID = %q, want empty: dev was reused, not lost", res.LostCredentialID)
	}
}
