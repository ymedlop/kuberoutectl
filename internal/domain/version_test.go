package domain

import "testing"

func TestNormalizeKubernetesVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"eks bare major.minor", "1.29", "1.29"},
		{"aks patch", "1.28.3", "1.28.3"},
		{"gke vendor suffix", "1.28.9-gke.1000000", "1.28.9"},
		{"pre-release suffix", "1.30.0-alpha.1", "1.30.0"},
		{"v prefix", "v1.30.0", "1.30.0"},
		{"upper v prefix", "V1.30.0", "1.30.0"},
		{"surrounding whitespace", "  1.27.7  ", "1.27.7"},
		{"empty", "", VersionUnknown},
		{"only whitespace", "   ", VersionUnknown},
		{"no digits", "stable", VersionUnknown},
		{"only v", "v", VersionUnknown},
		{"trailing junk dropped", "1.2.x", "1.2"},
		{"leading dot", ".1.2", VersionUnknown},
		{"embedded double dot", "1..2", VersionUnknown},
		{"only dots", "..1", VersionUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeKubernetesVersion(c.in); got != c.want {
				t.Errorf("NormalizeKubernetesVersion(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
