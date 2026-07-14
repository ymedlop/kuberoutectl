package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestAssignAliases_UniqueNamesUseBareSlug(t *testing.T) {
	targets := []domain.Target{
		{ID: "id-a", Name: "aks-prod-weu"},
		{ID: "id-b", Name: "eks-prod-frankfurt"},
	}
	AssignAliases(targets)
	if targets[0].Alias != "aks-prod-weu" {
		t.Errorf("alias[0] = %q, want aks-prod-weu", targets[0].Alias)
	}
	if targets[1].Alias != "eks-prod-frankfurt" {
		t.Errorf("alias[1] = %q, want eks-prod-frankfurt", targets[1].Alias)
	}
}

func TestAssignAliases_CollisionsGetStableSuffix(t *testing.T) {
	targets := []domain.Target{
		{ID: "id-a", Name: "prod"},
		{ID: "id-b", Name: "prod"},
	}
	AssignAliases(targets)
	if targets[0].Alias == targets[1].Alias {
		t.Fatalf("colliding names must get distinct aliases, both %q", targets[0].Alias)
	}
	for _, tg := range targets {
		if len(tg.Alias) <= len("prod-") || tg.Alias[:5] != "prod-" {
			t.Errorf("collision alias %q should be prod-<hash>", tg.Alias)
		}
	}

	// Deterministic and order-independent: reversing input yields the same
	// alias for the same ID.
	rev := []domain.Target{targets[1], targets[0]}
	for i := range rev {
		rev[i].Alias = ""
	}
	AssignAliases(rev)
	if rev[0].Alias != targets[1].Alias || rev[1].Alias != targets[0].Alias {
		t.Errorf("aliases not order-independent: %q/%q vs %q/%q",
			rev[0].Alias, rev[1].Alias, targets[1].Alias, targets[0].Alias)
	}
}

func TestAssignAliases_EmptyNameFallsBackToHash(t *testing.T) {
	targets := []domain.Target{{ID: "some-long-id", Name: ""}}
	AssignAliases(targets)
	if targets[0].Alias == "" {
		t.Fatal("empty-name target must still get an alias")
	}
}

func TestResolveTargetRef(t *testing.T) {
	targets := []domain.Target{
		{ID: "arn:very:long:id/aks-prod-weu", Name: "aks-prod-weu"},
		{ID: "arn:very:long:id/dup", Name: "dup"},
		{ID: "arn:other:long:id/dup", Name: "dup"},
	}
	AssignAliases(targets)

	// by full ID
	got, err := ResolveTargetRef(targets, "arn:very:long:id/aks-prod-weu")
	if err != nil || got.Name != "aks-prod-weu" {
		t.Fatalf("by ID: got %+v err %v", got, err)
	}
	// by alias
	got, err = ResolveTargetRef(targets, "aks-prod-weu")
	if err != nil || got.ID != "arn:very:long:id/aks-prod-weu" {
		t.Fatalf("by alias: got %+v err %v", got, err)
	}
	// ambiguous name
	if _, err := ResolveTargetRef(targets, "dup"); err == nil {
		t.Fatal("expected ambiguity error for duplicate name")
	}
	// unambiguous alias for a collided name resolves fine
	if _, err := ResolveTargetRef(targets, targets[1].Alias); err != nil {
		t.Fatalf("resolving collided target by its alias failed: %v", err)
	}
	// unknown
	if _, err := ResolveTargetRef(targets, "nope"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestListFilters(t *testing.T) {
	m := newMemStore()
	m.snap = domain.InventorySnapshot{Targets: []domain.Target{
		{ID: "az1", ProviderID: "azure", Name: "aks-prod", Region: "westeurope"},
		{ID: "aw1", ProviderID: "aws", Name: "eks-prod", Region: "eu-central-1"},
		{ID: "aw2", ProviderID: "aws", Name: "eks-lab", Region: "eu-central-1"},
	}}
	svc := NewTargetService(m)

	all, _ := svc.List(TargetFilter{})
	if len(all) != 3 {
		t.Fatalf("no filter: got %d want 3", len(all))
	}
	for _, tg := range all {
		if tg.Alias == "" {
			t.Errorf("List must assign aliases, %q has none", tg.ID)
		}
	}

	awsOnly, _ := svc.List(TargetFilter{Provider: "aws"})
	if len(awsOnly) != 2 {
		t.Fatalf("provider filter: got %d want 2", len(awsOnly))
	}

	sel, err := ParseSelector([]string{"region=eu-central-1"})
	if err != nil {
		t.Fatal(err)
	}
	inRegion, _ := svc.List(TargetFilter{Selector: &sel})
	if len(inRegion) != 2 {
		t.Fatalf("selector filter: got %d want 2", len(inRegion))
	}

	// combined: aws AND a specific name via selector
	sel2, _ := ParseSelector([]string{"region=eu-central-1"})
	combined, _ := svc.List(TargetFilter{Provider: "aws", Selector: &sel2})
	if len(combined) != 2 {
		t.Fatalf("combined filter: got %d want 2", len(combined))
	}
}
