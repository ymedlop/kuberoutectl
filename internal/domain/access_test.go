package domain

import "testing"

// The truth table from the spec, one case per cell. Only a *negative* answer
// depends on the authentication mode, and that asymmetry is the single thing a
// naive implementation gets wrong: it is tempting to read "absent" as "refused"
// and be right only under `api`.
//
// Written as an exhaustive table rather than a few examples because a wrong
// answer here is a confident lie — the operator is told a profile will be
// refused when nothing established that.
func TestAccessVerdictFor(t *testing.T) {
	cases := []struct {
		name   string
		listed bool
		check  AccessCheckMode
		want   AccessVerdict
	}{
		{"api, listed", true, AccessCheckAPI, AccessOperable},
		{"api, absent — the only cell that may say no", false, AccessCheckAPI, AccessNotOperable},

		{"api_and_config_map, listed", true, AccessCheckAPIAndConfigMap, AccessOperable},
		{"api_and_config_map, absent — aws-auth may still grant it", false, AccessCheckAPIAndConfigMap, AccessUnknown},

		// Under config_map no access entries exist at all, so "listed" cannot
		// happen. It is covered anyway: a verdict function that trusts its caller
		// to never pass an impossible combination is one refactor away from a lie.
		{"config_map, listed (impossible, must not crash)", true, AccessCheckConfigMap, AccessOperable},
		{"config_map, absent — entries do not apply", false, AccessCheckConfigMap, AccessUnknown},

		{"unavailable, absent — the call failed, nothing was established", false, AccessCheckUnavailable, AccessUnknown},
		{"not attempted, absent", false, "", AccessUnknown},
		{"a mode this build does not know", false, "SOMETHING_NEW", AccessUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccessVerdictFor(tc.listed, tc.check); got != tc.want {
				t.Errorf("AccessVerdictFor(%v, %q) = %q, want %q", tc.listed, tc.check, got, tc.want)
			}
		})
	}
}

// Edge case 2: a CONFIG_MAP cluster has no entries, so every credential is
// unknown. Rendering any of them as "not operable" would be a fabrication.
func TestCredentialAccess_ConfigMapClusterIsAllUnknown(t *testing.T) {
	tgt := Target{
		CredentialID:  "aws:ops",
		CredentialIDs: []CredentialID{"aws:ops", "aws:dev"},
		AccessCheck:   AccessCheckConfigMap,
	}
	for _, id := range tgt.CredentialIDs {
		if got := tgt.CredentialAccess(id); got != AccessUnknown {
			t.Errorf("CredentialAccess(%q) = %q, want %q", id, got, AccessUnknown)
		}
	}
}

// Under `api` the list is authoritative in both directions, which is what makes
// the primary-selection ranking meaningful.
func TestCredentialAccess_APIModeIsAuthoritativeBothWays(t *testing.T) {
	tgt := Target{
		CredentialID:          "aws:ops",
		CredentialIDs:         []CredentialID{"aws:ops", "aws:dev"},
		OperableCredentialIDs: []CredentialID{"aws:ops"},
		AccessCheck:           AccessCheckAPI,
	}
	if got := tgt.CredentialAccess("aws:ops"); got != AccessOperable {
		t.Errorf("listed credential = %q, want %q", got, AccessOperable)
	}
	if got := tgt.CredentialAccess("aws:dev"); got != AccessNotOperable {
		t.Errorf("absent credential under api = %q, want %q", got, AccessNotOperable)
	}
}

// AccessCheckMode.Valid recognises exactly the modes this build understands.
// A mode AWS adds later must read as unrecognised rather than silently
// behaving like `api`, since `api` is the only mode allowed to say "no".
func TestAccessCheckModeValid(t *testing.T) {
	for _, m := range []AccessCheckMode{AccessCheckAPI, AccessCheckAPIAndConfigMap, AccessCheckConfigMap, AccessCheckUnavailable} {
		if !m.Valid() {
			t.Errorf("%q must be recognised", m)
		}
	}
	for _, m := range []AccessCheckMode{"", "api_only", "API"} {
		if m.Valid() {
			t.Errorf("%q must not be recognised", m)
		}
	}
}
