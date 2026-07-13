package domain

import "testing"

func TestSelector_MatchLabelsExact(t *testing.T) {
	sel := LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	if !sel.Matches(map[string]string{"env": "prod", "team": "platform"}) {
		t.Error("expected match on exact env=prod")
	}
	if sel.Matches(map[string]string{"env": "lab"}) {
		t.Error("expected no match on env=lab")
	}
	if sel.Matches(map[string]string{"team": "platform"}) {
		t.Error("expected no match when key absent")
	}
}

func TestSelector_MatchAnyInList(t *testing.T) {
	sel := LabelSelector{MatchAny: map[string][]string{
		"region": {"westeurope", "eu-west-1"},
	}}
	if !sel.Matches(map[string]string{"region": "eu-west-1"}) {
		t.Error("expected match for region in list")
	}
	if sel.Matches(map[string]string{"region": "us-east-1"}) {
		t.Error("expected no match for region outside list")
	}
	if sel.Matches(map[string]string{}) {
		t.Error("expected no match when region key absent")
	}
}

func TestSelector_CombinedAllMustHold(t *testing.T) {
	sel := LabelSelector{
		MatchLabels: map[string]string{"env": "prod"},
		MatchAny:    map[string][]string{"region": {"westeurope"}},
	}
	if !sel.Matches(map[string]string{"env": "prod", "region": "westeurope"}) {
		t.Error("expected match when both constraints hold")
	}
	if sel.Matches(map[string]string{"env": "prod", "region": "us-east-1"}) {
		t.Error("expected no match when one constraint fails")
	}
}

func TestSelector_ZeroMatchesNothing(t *testing.T) {
	var sel LabelSelector
	if !sel.IsZero() {
		t.Fatal("expected zero selector to report IsZero")
	}
	if sel.Matches(map[string]string{"env": "prod"}) {
		t.Error("zero selector must not match everything")
	}
}
