package services

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain https", "https://h.example.com", "h.example.com"},
		{"trailing slash", "https://h.example.com/", "h.example.com"},
		{"uppercase host", "https://H.Example.COM", "h.example.com"},
		{"default https port stripped", "https://h.example.com:443", "h.example.com"},
		{"default http port stripped", "http://h.example.com:80", "h.example.com"},
		{"non-default port kept", "https://h.example.com:6443", "h.example.com:6443"},
		{"http and https compare equal (a)", "http://h.example.com", "h.example.com"},
		{"http and https compare equal (b)", "https://h.example.com", "h.example.com"},
		{"gcp post-build ip form", "https://35.40.50.60", "35.40.50.60"},
		{"ip with non-default port", "https://192.168.1.10:6443", "192.168.1.10:6443"},
		{"ipv6 with non-default port", "https://[2001:db8::1]:6443", "[2001:db8::1]:6443"},
		{"ipv6 default port stripped and bracketed", "https://[2001:db8::1]:443", "[2001:db8::1]"},
		{"ipv6 no port bracketed", "https://[2001:db8::1]", "[2001:db8::1]"},
		{"empty is empty", "", ""},
		{"whitespace is empty", "   ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeEndpoint(c.in); got != c.want {
				t.Errorf("normalizeEndpoint(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Regression: an IPv6 host must never collide with a differently-shaped
// (host, port) pair. Naive host+":"+port encoding made these two distinct
// endpoints hash to the same key, silently suppressing a real cluster.
func TestNormalizeEndpoint_IPv6NoCollision(t *testing.T) {
	a := normalizeEndpoint("https://[2001:db8::1]:6443") // host 2001:db8::1, port 6443
	b := normalizeEndpoint("https://[2001:db8::1:6443]") // host 2001:db8::1:6443, no port
	if a == b {
		t.Fatalf("distinct IPv6 endpoints collapsed to the same key: %q", a)
	}
}

func isKubeconfigOverlay(id domain.ProviderID) bool { return id == "kubeconfig" }

func targetIDs(ts []domain.Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t.ID)
	}
	return out
}

func TestSuppressOverlayDuplicates(t *testing.T) {
	const eks = "https://ABC123.gr7.eu-central-1.eks.amazonaws.com"

	t.Run("overlay duplicate is dropped, native kept", func(t *testing.T) {
		in := []domain.Target{
			{ID: "aws:eks", ProviderID: "aws", Endpoint: eks},
			{ID: "kubeconfig:context:eks", ProviderID: "kubeconfig", Endpoint: eks},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		got := targetIDs(out)
		if len(got) != 1 || got[0] != "aws:eks" {
			t.Fatalf("expected only native aws:eks, got %v", got)
		}
	})

	t.Run("scheme and slash differences still match", func(t *testing.T) {
		in := []domain.Target{
			{ID: "aws:eks", ProviderID: "aws", Endpoint: "https://h.example.com"},
			{ID: "kubeconfig:context:eks", ProviderID: "kubeconfig", Endpoint: "http://h.example.com/"},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		if ids := targetIDs(out); len(ids) != 1 || ids[0] != "aws:eks" {
			t.Fatalf("expected native only, got %v", ids)
		}
	})

	t.Run("two native targets sharing an endpoint are both kept", func(t *testing.T) {
		in := []domain.Target{
			{ID: "aws:a", ProviderID: "aws", Endpoint: eks},
			{ID: "gcp:b", ProviderID: "gcp", Endpoint: eks},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		if len(out) != 2 {
			t.Fatalf("native+native must not dedup, got %v", targetIDs(out))
		}
	})

	t.Run("overlay-only sharing an endpoint with no native owner are both kept", func(t *testing.T) {
		in := []domain.Target{
			{ID: "kubeconfig:context:a", ProviderID: "kubeconfig", Endpoint: eks},
			{ID: "kubeconfig:context:b", ProviderID: "kubeconfig", Endpoint: eks},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		if len(out) != 2 {
			t.Fatalf("overlay-only must not dedup without a native owner, got %v", targetIDs(out))
		}
	})

	t.Run("empty endpoints never collapse", func(t *testing.T) {
		in := []domain.Target{
			{ID: "aws:noendpoint", ProviderID: "aws", Endpoint: ""},
			{ID: "kubeconfig:context:homelab", ProviderID: "kubeconfig", Endpoint: ""},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		if len(out) != 2 {
			t.Fatalf("empty endpoints must never be treated as duplicates, got %v", targetIDs(out))
		}
	})

	t.Run("order is preserved", func(t *testing.T) {
		in := []domain.Target{
			{ID: "a", ProviderID: "aws", Endpoint: "https://a"},
			{ID: "kubeconfig:context:dup", ProviderID: "kubeconfig", Endpoint: "https://a"},
			{ID: "b", ProviderID: "gcp", Endpoint: "https://b"},
			{ID: "kubeconfig:context:keep", ProviderID: "kubeconfig", Endpoint: "https://c"},
		}
		out := suppressOverlayDuplicates(in, isKubeconfigOverlay)
		got := targetIDs(out)
		want := []string{"a", "b", "kubeconfig:context:keep"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order not preserved: got %v, want %v", got, want)
			}
		}
	})
}
