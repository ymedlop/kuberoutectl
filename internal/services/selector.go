package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// SelectorEngine evaluates label selectors against targets. It is a thin,
// stateless wrapper over domain.LabelSelector that always evaluates against a
// target's EffectiveLabels (user labels overriding system labels), so both
// discovered and user metadata are selectable.
type SelectorEngine struct{}

// NewSelectorEngine returns a SelectorEngine.
func NewSelectorEngine() *SelectorEngine { return &SelectorEngine{} }

// Matches reports whether the target satisfies the selector. It evaluates
// against SelectionLabels so both real labels and structured attribute aliases
// (region, platform, provider, ...) are selectable.
func (SelectorEngine) Matches(sel domain.LabelSelector, t domain.Target) bool {
	return sel.Matches(t.SelectionLabels())
}

// Filter returns the targets that match the selector, preserving input order.
func (e SelectorEngine) Filter(sel domain.LabelSelector, targets []domain.Target) []domain.Target {
	out := make([]domain.Target, 0, len(targets))
	for _, t := range targets {
		if e.Matches(sel, t) {
			out = append(out, t)
		}
	}
	return out
}

// inClause matches `key in [a, b, c]` or `key in (a, b, c)`.
var inClause = regexp.MustCompile(`^\s*([^\s]+)\s+in\s+[\[(]([^\])]*)[\])]\s*$`)

// ParseSelector builds a LabelSelector from CLI clauses. Each clause is either:
//
//   - one or more comma-separated equalities: "env=prod" or "env=prod,team=platform"
//   - a single in-list: "region in [westeurope, eu-west-1]"
//
// Clauses may be supplied as repeated --selector flags or comma-joined for the
// equality form. This is the deliberately small milestone-1 grammar — no
// boolean expression language.
func ParseSelector(clauses []string) (domain.LabelSelector, error) {
	sel := domain.LabelSelector{}
	for _, raw := range clauses {
		clause := strings.TrimSpace(raw)
		if clause == "" {
			continue
		}
		if m := inClause.FindStringSubmatch(clause); m != nil {
			key := strings.TrimSpace(m[1])
			if key == "" {
				return domain.LabelSelector{}, fmt.Errorf("selector %q has empty key", clause)
			}
			var vals []string
			for _, v := range strings.Split(m[2], ",") {
				if v = strings.TrimSpace(v); v != "" {
					vals = append(vals, v)
				}
			}
			if len(vals) == 0 {
				return domain.LabelSelector{}, fmt.Errorf("selector %q has an empty value list", clause)
			}
			if sel.MatchAny == nil {
				sel.MatchAny = map[string][]string{}
			}
			sel.MatchAny[key] = vals
			continue
		}
		// Equality form, possibly comma-joined.
		for _, part := range strings.Split(clause, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			k, v, ok := strings.Cut(part, "=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if !ok || k == "" {
				return domain.LabelSelector{}, fmt.Errorf("invalid selector clause %q: want key=value or `key in [..]`", part)
			}
			if sel.MatchLabels == nil {
				sel.MatchLabels = map[string]string{}
			}
			sel.MatchLabels[k] = v
		}
	}
	if sel.IsZero() {
		return domain.LabelSelector{}, fmt.Errorf("selector is empty")
	}
	return sel, nil
}
