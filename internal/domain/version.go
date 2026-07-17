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
// "1.29" and it stays "1.29" (no padding). Anything without a leading digit
// (empty, "stable") normalizes to VersionUnknown.
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
	if core == "" || !strings.ContainsAny(core, "0123456789") {
		return VersionUnknown
	}
	return core
}
