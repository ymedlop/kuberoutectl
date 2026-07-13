package services

import (
	"reflect"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestParseSelector_Equality(t *testing.T) {
	sel, err := ParseSelector([]string{"env=prod,team=platform"})
	if err != nil {
		t.Fatalf("ParseSelector: %v", err)
	}
	want := map[string]string{"env": "prod", "team": "platform"}
	if !reflect.DeepEqual(sel.MatchLabels, want) {
		t.Errorf("MatchLabels = %+v, want %+v", sel.MatchLabels, want)
	}
}

func TestParseSelector_InList(t *testing.T) {
	sel, err := ParseSelector([]string{"region in [westeurope, eu-west-1]"})
	if err != nil {
		t.Fatalf("ParseSelector: %v", err)
	}
	want := []string{"westeurope", "eu-west-1"}
	if !reflect.DeepEqual(sel.MatchAny["region"], want) {
		t.Errorf("MatchAny[region] = %+v, want %+v", sel.MatchAny["region"], want)
	}
}

func TestParseSelector_RepeatedFlagsCombine(t *testing.T) {
	sel, err := ParseSelector([]string{"env=prod", "region in (westeurope)"})
	if err != nil {
		t.Fatalf("ParseSelector: %v", err)
	}
	if sel.MatchLabels["env"] != "prod" || len(sel.MatchAny["region"]) != 1 {
		t.Errorf("combined selector wrong: %+v", sel)
	}
}

func TestParseSelector_Errors(t *testing.T) {
	for _, in := range [][]string{
		{""},             // empty -> no constraints
		{"noequalsign"},  // not k=v, not in-list
		{"=novalue"},     // empty key
		{"region in []"}, // empty list
	} {
		if _, err := ParseSelector(in); err == nil {
			t.Errorf("expected error for %+v", in)
		}
	}
}

func TestSelectorEngine_FilterUsesEffectiveLabels(t *testing.T) {
	eng := NewSelectorEngine()
	targets := []domain.Target{
		{ID: "a", SystemLabels: map[string]string{domain.LabelRegion: "westeurope"}, UserLabels: map[string]string{"env": "prod"}},
		{ID: "b", SystemLabels: map[string]string{domain.LabelRegion: "eu-west-1"}, UserLabels: map[string]string{"env": "lab"}},
	}
	sel := domain.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	got := eng.Filter(sel, targets)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("expected only target a, got %+v", got)
	}

	// A selector on a system label must also work.
	selSys := domain.LabelSelector{MatchAny: map[string][]string{domain.LabelRegion: {"eu-west-1"}}}
	got = eng.Filter(selSys, targets)
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("expected only target b via system label, got %+v", got)
	}
}

// Bare structured aliases (region, platform, ...) must be selectable, matching
// the README's `region in [...]` examples.
func TestSelectorEngine_BareStructuredAlias(t *testing.T) {
	eng := NewSelectorEngine()
	targets := []domain.Target{
		{ID: "a", Region: "westeurope", Platform: "aks"},
		{ID: "b", Region: "eu-west-1", Platform: "eks"},
	}
	sel := domain.LabelSelector{MatchAny: map[string][]string{"region": {"westeurope"}}}
	got := eng.Filter(sel, targets)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("expected target a via bare region alias, got %+v", got)
	}

	// A user label named "region" must override the structured alias.
	targets[1].UserLabels = map[string]string{"region": "westeurope"}
	got = eng.Filter(sel, targets)
	if len(got) != 2 {
		t.Fatalf("expected user label to override alias, got %+v", got)
	}
}
