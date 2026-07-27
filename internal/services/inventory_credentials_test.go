package services

import (
	"reflect"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func storeForCredentialJoin() *memStore {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{
		Credentials: []domain.Credential{
			{ID: "aws:dev", ProviderID: "aws", Name: "dev", Health: domain.HealthExpired},
			{ID: "aws:ops", ProviderID: "aws", Name: "ops", Health: domain.HealthValid},
			{ID: "gcp:account:me", ProviderID: "gcp", Name: "me", Health: domain.HealthValid},
		},
		Targets: []domain.Target{
			{
				ID: "t-multi", ProviderID: "aws", Name: "eks-prod",
				CredentialID:  "aws:ops",
				CredentialIDs: []domain.CredentialID{"aws:ops", "aws:dev"},
			},
			{ID: "t-single", ProviderID: "gcp", Name: "gke-prod", CredentialID: "gcp:account:me"},
		},
	}
	return m
}

// The join preserves CredentialIDs order, so "primary first" survives to the
// display layer without the caller re-sorting.
func TestResolveWithCredentials_PrimaryFirst(t *testing.T) {
	svc := NewTargetService(storeForCredentialJoin())

	got, err := svc.ResolveWithCredentials("eks-prod")
	if err != nil {
		t.Fatalf("ResolveWithCredentials: %v", err)
	}
	if want := []string{"ops", "dev"}; !reflect.DeepEqual(got.CredentialNames(), want) {
		t.Errorf("CredentialNames() = %v, want %v", got.CredentialNames(), want)
	}
	if got.Credentials[1].Health != domain.HealthExpired {
		t.Errorf("dev health = %q, want it joined live from the snapshot", got.Credentials[1].Health)
	}
}

// A target with no CredentialIDs — every single-credential provider, and every
// snapshot written before the field existed — still yields its one credential.
func TestResolveWithCredentials_SingleCredentialTarget(t *testing.T) {
	svc := NewTargetService(storeForCredentialJoin())

	got, err := svc.ResolveWithCredentials("gke-prod")
	if err != nil {
		t.Fatalf("ResolveWithCredentials: %v", err)
	}
	if want := []string{"me"}; !reflect.DeepEqual(got.CredentialNames(), want) {
		t.Errorf("CredentialNames() = %v, want %v", got.CredentialNames(), want)
	}
}

// A credential a resync dropped is simply absent from the join rather than
// appearing as a blank entry.
func TestResolveWithCredentials_SkipsMissingCredential(t *testing.T) {
	store := storeForCredentialJoin()
	store.snap.Credentials = store.snap.Credentials[:1] // keep only aws:dev
	svc := NewTargetService(store)

	got, err := svc.ResolveWithCredentials("eks-prod")
	if err != nil {
		t.Fatalf("ResolveWithCredentials: %v", err)
	}
	if want := []string{"dev"}; !reflect.DeepEqual(got.CredentialNames(), want) {
		t.Errorf("CredentialNames() = %v, want %v — the missing one must be skipped", got.CredentialNames(), want)
	}
}

// ListWithCredentials joins every row, so `target list` can show a PROFILES
// column without a per-target snapshot load.
func TestListWithCredentials_JoinsEveryRow(t *testing.T) {
	svc := NewTargetService(storeForCredentialJoin())

	rows, err := svc.ListWithCredentials(TargetFilter{})
	if err != nil {
		t.Fatalf("ListWithCredentials: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	byName := map[string][]string{}
	for _, r := range rows {
		byName[r.Target.Name] = r.CredentialNames()
	}
	if want := []string{"ops", "dev"}; !reflect.DeepEqual(byName["eks-prod"], want) {
		t.Errorf("eks-prod = %v, want %v", byName["eks-prod"], want)
	}
	if want := []string{"me"}; !reflect.DeepEqual(byName["gke-prod"], want) {
		t.Errorf("gke-prod = %v, want %v", byName["gke-prod"], want)
	}
}

// The filter must still apply — the join is an addition to List, not a bypass.
func TestListWithCredentials_HonoursFilter(t *testing.T) {
	svc := NewTargetService(storeForCredentialJoin())

	rows, err := svc.ListWithCredentials(TargetFilter{Provider: "gcp"})
	if err != nil {
		t.Fatalf("ListWithCredentials: %v", err)
	}
	if len(rows) != 1 || rows[0].Target.Name != "gke-prod" {
		t.Fatalf("got %d rows (%+v), want only the gcp target", len(rows), rows)
	}
}
