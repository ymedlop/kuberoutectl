package domain

import "strings"

// VersionUnknown is the sentinel a target carries when its Kubernetes server
// version could not be determined — a provider whose data source has no version
// (kubeconfig) or a value that did not parse. It is a real, displayable value,
// never the empty string, so "missing" reads the same everywhere.
const VersionUnknown = "unknown"

// NormalizeKubernetesVersion reduces a provider-reported server version to its
// dotted-numeric core: it trims whitespace, drops a leading v/V, and keeps only
// the leading run of digits and dots — discarding vendor and pre-release
// suffixes (GKE's "-gke.1000000", a channel's "-alpha.1"). EKS reports a bare
// "1.29" and it stays "1.29" (no padding). The core must be a well-formed
// dotted-numeric string (every dot-separated segment a non-empty digit run);
// anything else — "stable", "" , ".1.2", "1..2" — normalizes to VersionUnknown.
func NormalizeKubernetesVersion(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	end := 0
	for end < len(s) {
		c := s[end]
		if (c >= '0' && c <= '9') || c == '.' {
			end++
			continue
		}
		break
	}
	core := strings.TrimRight(s[:end], ".")
	if core == "" {
		return VersionUnknown
	}
	// The char filter above admits only digits and dots, so a segment is either
	// digits or empty; requiring every segment non-empty rejects a leading dot
	// (".1.2") or an embedded double dot ("1..2") that would otherwise slip
	// through as a bogus "core".
	for _, seg := range strings.Split(core, ".") {
		if seg == "" {
			return VersionUnknown
		}
	}
	return core
}
