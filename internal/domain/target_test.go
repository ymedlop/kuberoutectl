package domain

import "testing"

// SelectionLabels must always emit both visibility keys (in both states) so the
// default-hide rule's HasKey check and hidden=false matching are reliable.
func TestSelectionLabels_VisibleHidden(t *testing.T) {
	visible := Target{ProviderID: "aws", Region: "eu-central-1"}.SelectionLabels()
	if visible["visible"] != "true" || visible["hidden"] != "false" {
		t.Errorf("visible target: got visible=%q hidden=%q", visible["visible"], visible["hidden"])
	}

	hidden := Target{ProviderID: "aws", Hidden: true}.SelectionLabels()
	if hidden["visible"] != "false" || hidden["hidden"] != "true" {
		t.Errorf("hidden target: got visible=%q hidden=%q", hidden["visible"], hidden["hidden"])
	}

	// Present unconditionally, even for a visible target.
	if _, ok := visible["hidden"]; !ok {
		t.Error("hidden key must always be present")
	}
}

// A stray user label named "hidden"/"visible" (from a hand-edited state file or
// a pre-reservation install) must not shadow the computed visibility.
func TestSelectionLabels_ComputedVisibilityIsAuthoritative(t *testing.T) {
	tg := Target{
		ProviderID: "aws",
		Hidden:     true,
		UserLabels: map[string]string{"hidden": "false", "visible": "true"},
	}
	sl := tg.SelectionLabels()
	if sl["hidden"] != "true" || sl["visible"] != "false" {
		t.Errorf("computed visibility must win over a shadowing user label: got hidden=%q visible=%q", sl["hidden"], sl["visible"])
	}
}

func TestLabelSelector_HasKey(t *testing.T) {
	sel := LabelSelector{MatchLabels: map[string]string{"hidden": "true"}}
	if !sel.HasKey("hidden") {
		t.Error("expected HasKey(hidden) via MatchLabels")
	}
	if sel.HasKey("visible") {
		t.Error("did not expect HasKey(visible)")
	}
	viaAny := LabelSelector{MatchAny: map[string][]string{"visible": {"true"}}}
	if !viaAny.HasKey("visible") {
		t.Error("expected HasKey(visible) via MatchAny")
	}
	if (LabelSelector{}).HasKey("hidden") {
		t.Error("zero selector has no keys")
	}
}
