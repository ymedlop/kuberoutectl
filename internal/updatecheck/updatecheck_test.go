package updatecheck

import "testing"

// The comparison must be numeric per field, not lexical. `"1.10.0" < "1.9.0"` as
// strings, so a string compare stops offering upgrades at the tenth minor
// release and does it silently — the version numbers still look plausible.
//
// Anything that is not a plain stable release must yield ok=false rather than a
// guess: a dev or snapshot build has no comparable version, and offering a
// pre-release to someone on a stable one is exactly what the spec forbids.
func TestNewer(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		newer, ok       bool
	}{
		{name: "patch bump", current: "1.0.0", latest: "1.0.1", newer: true, ok: true},
		{name: "minor bump", current: "1.0.0", latest: "1.1.0", newer: true, ok: true},
		{name: "major bump", current: "1.9.9", latest: "2.0.0", newer: true, ok: true},
		{name: "identical", current: "1.1.0", latest: "1.1.0", newer: false, ok: true},
		{name: "current is ahead", current: "1.2.0", latest: "1.1.0", newer: false, ok: true},

		// The case a lexical compare gets wrong, in both directions.
		{name: "1.10.0 is newer than 1.9.0", current: "1.9.0", latest: "1.10.0", newer: true, ok: true},
		{name: "1.9.0 is not newer than 1.10.0", current: "1.10.0", latest: "1.9.0", newer: false, ok: true},
		{name: "patch 10 beats patch 9", current: "1.0.9", latest: "1.0.10", newer: true, ok: true},

		{name: "v prefix on latest", current: "1.0.0", latest: "v1.1.0", newer: true, ok: true},
		{name: "v prefix on both", current: "v1.0.0", latest: "v1.1.0", newer: true, ok: true},

		// No verdict at all. Each of these would otherwise nag forever or crash.
		{name: "dev build", current: "dev", latest: "1.1.0"},
		{name: "snapshot build", current: "0.0.0-snapshot-abc1234", latest: "1.1.0"},
		{name: "current is a pre-release", current: "1.2.0-rc.1", latest: "1.2.0"},
		{name: "latest is a pre-release", current: "1.1.0", latest: "1.2.0-rc.1"},
		{name: "empty current", current: "", latest: "1.1.0"},
		{name: "empty latest", current: "1.0.0", latest: ""},
		{name: "too few fields", current: "1.2", latest: "1.3"},
		{name: "too many fields", current: "1.2.3.4", latest: "1.2.4"},
		{name: "non-numeric field", current: "1.x.0", latest: "1.1.0"},
		{name: "build metadata", current: "1.0.0+build5", latest: "1.1.0"},
		{name: "not a version at all", current: "kuberoutectl", latest: "1.1.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newer, ok := Newer(tc.current, tc.latest)
			if ok != tc.ok {
				t.Fatalf("Newer(%q, %q) ok = %v, want %v", tc.current, tc.latest, ok, tc.ok)
			}
			if newer != tc.newer {
				t.Errorf("Newer(%q, %q) newer = %v, want %v", tc.current, tc.latest, newer, tc.newer)
			}
		})
	}
}

// Enabled decides, before any HTTP client exists, whether a check may happen at
// all. Both suppression conditions live here so that "no row" and "no request"
// are one guarantee rather than two that can drift apart.
func TestEnabled(t *testing.T) {
	cases := []struct {
		name    string
		version string
		env     string // "" means unset
		setEnv  bool
		want    bool
	}{
		{name: "stable release, no opt-out", version: "1.0.0", want: true},
		{name: "stable release with v prefix", version: "v1.0.0", want: true},

		{name: "opt-out set to 1", version: "1.0.0", env: "1", setEnv: true},
		{name: "opt-out set to true", version: "1.0.0", env: "true", setEnv: true},
		// Any non-empty value disables. Someone writing =0 means "off"; reading it
		// as a boolean and enabling the check would be the opposite of their
		// intent, on the one setting whose whole purpose is to stop a request.
		{name: "opt-out set to 0 still disables", version: "1.0.0", env: "0", setEnv: true},
		{name: "opt-out set to empty does not disable", version: "1.0.0", env: "", setEnv: true, want: true},

		{name: "dev build", version: "dev"},
		{name: "snapshot build", version: "0.0.0-snapshot-abc1234"},
		{name: "pre-release build", version: "1.2.0-rc.1"},
		{name: "empty version", version: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				t.Setenv(EnvDisable, tc.env)
			}
			if got := Enabled(tc.version); got != tc.want {
				t.Errorf("Enabled(%q) with %s=%q = %v, want %v", tc.version, EnvDisable, tc.env, got, tc.want)
			}
		})
	}
}
