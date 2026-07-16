package domain

// LabelSelector is the milestone-1 selection model over target labels. It is
// intentionally tiny — no boolean expression language — because the MVP only
// needs exact match and simple "in list" semantics:
//
//	MatchLabels: {"env": "prod"}                 -> env == prod
//	MatchAny:    {"region": ["westeurope","eu-west-1"]} -> region in that set
//
// A selector matches a target when ALL MatchLabels entries match AND every
// MatchAny key has the target's value present in its allowed list.
type LabelSelector struct {
	MatchLabels map[string]string   `json:"match_labels,omitempty"`
	MatchAny    map[string][]string `json:"match_any,omitempty"`
}

// IsZero reports whether the selector has no constraints. A zero selector
// matches nothing on its own (a collection with a zero selector relies on
// StaticIDs), which is safer than silently matching everything.
func (s LabelSelector) IsZero() bool {
	return len(s.MatchLabels) == 0 && len(s.MatchAny) == 0
}

// HasKey reports whether the selector constrains the given key (via either
// MatchLabels or MatchAny). The default-hide rule uses it to detect a selector
// that already constrains visibility (visible/hidden) and so should not be
// auto-filtered.
func (s LabelSelector) HasKey(key string) bool {
	if _, ok := s.MatchLabels[key]; ok {
		return true
	}
	_, ok := s.MatchAny[key]
	return ok
}

// Matches evaluates the selector against a label set. Evaluation is
// deterministic and side-effect free.
func (s LabelSelector) Matches(labels map[string]string) bool {
	if s.IsZero() {
		return false
	}
	for k, want := range s.MatchLabels {
		if labels[k] != want {
			return false
		}
	}
	for k, allowed := range s.MatchAny {
		got, ok := labels[k]
		if !ok {
			return false
		}
		found := false
		for _, a := range allowed {
			if got == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
