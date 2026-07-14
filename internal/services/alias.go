package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// AssignAliases fills in a short, stable, unique Alias on each target, in place.
//
// The alias is derived from the target name (slugified). When a slug is unique
// across the given set it is used as-is; when several targets share a slug they
// are each disambiguated with a short hash of their (unique) ID. The result is
// deterministic and order-independent: the same set of targets always yields
// the same aliases, so an alias printed by `target list` is safe to paste into
// `target use`.
//
// Aliases are computed on read rather than persisted, so they work on caches
// written before this existed and never drift from the snapshot they describe.
func AssignAliases(targets []domain.Target) {
	base := make([]string, len(targets))
	counts := map[string]int{}
	for i, t := range targets {
		b := slugify(t.Name)
		if b == "" {
			b = shortHash(string(t.ID))
		}
		base[i] = b
		counts[b]++
	}
	for i := range targets {
		if counts[base[i]] > 1 {
			targets[i].Alias = base[i] + "-" + shortHash(string(targets[i].ID))
		} else {
			targets[i].Alias = base[i]
		}
	}
}

// ResolveTargetRef finds a target by a user-supplied reference, which may be its
// full ID, its alias, or its name. Resolution order is ID, then alias, then
// name; the ID path keeps existing scripts (and the full resource IDs) working,
// while alias/name give operators a short handle. A name that matches more than
// one target is reported as ambiguous with the aliases to pick from.
//
// Callers should have run AssignAliases over the same slice first.
func ResolveTargetRef(targets []domain.Target, ref string) (domain.Target, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return domain.Target{}, fmt.Errorf("empty target reference")
	}
	for _, t := range targets {
		if string(t.ID) == ref {
			return t, nil
		}
	}
	for _, t := range targets {
		if t.Alias == ref {
			return t, nil
		}
	}
	var byName []domain.Target
	for _, t := range targets {
		if strings.EqualFold(t.Name, ref) {
			byName = append(byName, t)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return domain.Target{}, fmt.Errorf("no target matches %q (try an alias or ID from `kuberoutectl target list`)", ref)
	default:
		aliases := make([]string, 0, len(byName))
		for _, t := range byName {
			aliases = append(aliases, t.Alias)
		}
		return domain.Target{}, fmt.Errorf("target %q is ambiguous; use one of: %s", ref, strings.Join(aliases, ", "))
	}
}

// slugify reduces a name to a lowercase [a-z0-9-] handle: runs of other
// characters collapse to a single '-', and leading/trailing '-' are trimmed.
func slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// shortHash returns the first 6 hex chars of the SHA-256 of s: enough to
// disambiguate colliding names while staying short and stable.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}
