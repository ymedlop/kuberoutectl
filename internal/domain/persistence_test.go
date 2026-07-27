package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// A target reachable through several credentials records all of them, primary
// first, while CredentialID keeps naming the primary. Readers written before
// multi-credential targets existed keep working off CredentialID alone.
func TestTargetCredentialIDsRoundTrip(t *testing.T) {
	in := Target{
		ID:            "arn:aws:eks:eu-west-1:1234:cluster/prod",
		ProviderID:    "aws",
		CredentialID:  "aws:ops",
		CredentialIDs: []CredentialID{"aws:ops", "aws:dev"},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Target
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := len(out.CredentialIDs), 2; got != want {
		t.Fatalf("CredentialIDs length = %d, want %d", got, want)
	}
	if out.CredentialIDs[0] != out.CredentialID {
		t.Errorf("first CredentialIDs entry = %q, want it to equal CredentialID %q",
			out.CredentialIDs[0], out.CredentialID)
	}
}

// A single-credential target must not carry the key at all, so snapshots for
// providers with exactly one way in stay byte-identical to today's.
func TestTargetSingleCredentialOmitsCredentialIDs(t *testing.T) {
	data, err := json.Marshal(Target{ID: "gcp:project:eu:cluster", CredentialID: "gcp:account:user"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "credential_ids") {
		t.Errorf("credential_ids must be omitted when empty, got %s", data)
	}
}

// The upgrade must not require a migration: a snapshot written before
// CredentialIDs existed has no such key, and decoding it yields a nil slice —
// which every reader is required to treat as "just the one in CredentialID".
func TestTargetPreUpgradeSnapshotDecodes(t *testing.T) {
	const preUpgrade = `{
		"id": "arn:aws:eks:eu-west-1:1234:cluster/prod",
		"provider_id": "aws",
		"credential_id": "aws:default",
		"health": "valid"
	}`
	var out Target
	if err := json.Unmarshal([]byte(preUpgrade), &out); err != nil {
		t.Fatalf("unmarshal pre-upgrade target: %v", err)
	}
	if out.CredentialIDs != nil {
		t.Errorf("CredentialIDs = %v, want nil for a pre-upgrade target", out.CredentialIDs)
	}
	if out.CredentialID != "aws:default" {
		t.Errorf("CredentialID = %q, want it preserved", out.CredentialID)
	}
}

// The selection remembers which access path it was activated through, so
// `current` can report how the kubeconfig was actually written.
func TestSelectionCredentialIDRoundTrip(t *testing.T) {
	data, err := json.Marshal(Selection{TargetID: "t1", CredentialID: "aws:ops"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Selection
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.CredentialID != "aws:ops" {
		t.Errorf("CredentialID = %q, want %q", out.CredentialID, "aws:ops")
	}
}

// A selection written before the field existed decodes to the empty value,
// which callers resolve as "the primary" — no migration step, no version bump.
func TestSelectionPreUpgradeDecodesAsPrimary(t *testing.T) {
	var out Selection
	if err := json.Unmarshal([]byte(`{"target_id":"t1","updated_at":"2026-07-18T16:01:20Z"}`), &out); err != nil {
		t.Fatalf("unmarshal pre-upgrade selection: %v", err)
	}
	if out.CredentialID != "" {
		t.Errorf("CredentialID = %q, want empty (meaning primary)", out.CredentialID)
	}
	if out.TargetID != "t1" {
		t.Errorf("TargetID = %q, want it preserved", out.TargetID)
	}
}

// An empty selection must not serialize the key, keeping state files for
// single-credential providers unchanged.
func TestSelectionOmitsEmptyCredentialID(t *testing.T) {
	data, err := json.Marshal(Selection{TargetID: "t1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "credential_id") {
		t.Errorf("credential_id must be omitted when empty, got %s", data)
	}
}

// LabelCredential lives in the reserved system namespace like every other
// discovery-owned key, so a user label can never shadow it.
func TestLabelCredentialIsReserved(t *testing.T) {
	if !strings.HasPrefix(LabelCredential, SystemLabelPrefix) {
		t.Fatalf("LabelCredential = %q, want the %q prefix", LabelCredential, SystemLabelPrefix)
	}
	if err := ValidateUserLabelKey(LabelCredential); err == nil {
		t.Errorf("ValidateUserLabelKey(%q) = nil, want a rejection", LabelCredential)
	}
}
