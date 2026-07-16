package services

import (
	"net/url"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// normalizeEndpoint reduces a target's API-server endpoint to a comparable
// host[:port] key, so the same physical cluster compares equal however it was
// discovered. Rules:
//
//   - scheme is dropped from the key, so http and https on the same host:port
//     compare equal;
//   - the host is lower-cased;
//   - a port equal to the scheme's standard default (443 for https, 80 for
//     http) is stripped; any other explicit port — notably 6443 — is kept and
//     compared literally;
//   - path/query/trailing slash are ignored.
//
// An empty or unparseable endpoint yields "", which callers treat as
// "no key" — two endpoint-less targets are never considered duplicates.
func normalizeEndpoint(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Ensure a scheme delimiter so url.Parse populates Host, not Path.
	if !strings.Contains(s, "://") {
		s = "//" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ""
	}
	port := u.Port()
	switch strings.ToLower(u.Scheme) {
	case "https":
		if port == "443" {
			port = ""
		}
	case "http":
		if port == "80" {
			port = ""
		}
	}
	if port != "" {
		return host + ":" + port
	}
	return host
}

// suppressOverlayDuplicates drops overlay-provider targets whose endpoint is
// already owned by a non-overlay (native) target, so a natively-discovered
// cluster wins over its kubeconfig-context shadow. It is pure: isOverlay is a
// predicate (not a registry), and input order is preserved. Running it over the
// whole merged snapshot on every sync makes the outcome independent of the
// order providers were synced in.
func suppressOverlayDuplicates(targets []domain.Target, isOverlay func(domain.ProviderID) bool) []domain.Target {
	nativeEndpoints := make(map[string]bool)
	for _, t := range targets {
		if isOverlay(t.ProviderID) {
			continue
		}
		if key := normalizeEndpoint(t.Endpoint); key != "" {
			nativeEndpoints[key] = true
		}
	}
	if len(nativeEndpoints) == 0 {
		return targets
	}

	out := make([]domain.Target, 0, len(targets))
	for _, t := range targets {
		if isOverlay(t.ProviderID) {
			if key := normalizeEndpoint(t.Endpoint); key != "" && nativeEndpoints[key] {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}
