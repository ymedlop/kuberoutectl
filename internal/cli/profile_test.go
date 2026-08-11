package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/cache/jsonstore"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// profileTestApp seeds one cluster reachable two ways (ops primary, dev
// expired) plus one single-credential target, so conditional rendering can be
// checked in both directions.
func profileTestApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	a := &app{registry: providers.NewRegistry(), store: jsonstore.New(dir, dir), output: formatText}
	snap := domain.InventorySnapshot{
		Credentials: []domain.Credential{
			{ID: "aws:ops", ProviderID: "aws", Name: "ops", Health: domain.HealthValid, ActionHint: domain.ActionUse},
			{ID: "aws:dev", ProviderID: "aws", Name: "dev", Health: domain.HealthExpired, ActionHint: domain.ActionRenew},
			{ID: "gcp:account:me", ProviderID: "gcp", Name: "me", Health: domain.HealthValid},
		},
		Targets: []domain.Target{
			{
				ID: "t-multi", ProviderID: "aws", Name: "eks-prod", Platform: "eks", Region: "eu-central-1",
				Health: domain.HealthValid, CredentialID: "aws:ops",
				CredentialIDs: []domain.CredentialID{"aws:ops", "aws:dev"},
			},
			{
				ID: "t-single", ProviderID: "gcp", Name: "gke-prod", Platform: "gke", Region: "europe-west1",
				Health: domain.HealthValid, CredentialID: "gcp:account:me",
			},
		},
	}
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return a
}

// singleCredentialApp has no target with a choice, so the PROFILES column must
// not appear — same rule the HIDDEN column already follows.
func singleCredentialApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	a := &app{registry: providers.NewRegistry(), store: jsonstore.New(dir, dir), output: formatText}
	snap := domain.InventorySnapshot{
		Credentials: []domain.Credential{{ID: "gcp:account:me", ProviderID: "gcp", Name: "me"}},
		Targets: []domain.Target{{
			ID: "t-single", ProviderID: "gcp", Name: "gke-prod", Platform: "gke",
			Health: domain.HealthValid, CredentialID: "gcp:account:me",
		}},
	}
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return a
}

func TestTargetList_ProfilesColumnShownOnlyWhenThereIsAChoice(t *testing.T) {
	out, err := runCmd(profileTestApp(t).targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if !strings.Contains(out, "PROFILES") {
		t.Errorf("expected a PROFILES column, got:\n%s", out)
	}
	if !strings.Contains(out, "ops,dev") {
		t.Errorf("expected the reaching profiles primary-first, got:\n%s", out)
	}

	out, err = runCmd(singleCredentialApp(t).targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if strings.Contains(out, "PROFILES") {
		t.Errorf("PROFILES must be absent when no target has a choice, got:\n%s", out)
	}
}

// Health is joined per credential, so an expired alternative reads as expired
// even though the target itself reports the primary's health.
func TestTargetInspect_ListsEachCredentialWithItsOwnHealth(t *testing.T) {
	out, err := runCmd(profileTestApp(t).targetInspectCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if !strings.Contains(out, "(primary)") {
		t.Errorf("expected the primary to be marked, got:\n%s", out)
	}
	for _, want := range []string{"ops", "valid", "dev", "expired", "renew"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the breakdown, got:\n%s", want, out)
		}
	}

	// Nothing to choose: no breakdown, so the common case stays uncluttered.
	out, err = runCmd(profileTestApp(t).targetInspectCmd(), "", "gke-prod")
	if err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if strings.Contains(out, "profile") {
		t.Errorf("single-credential target must not list profiles, got:\n%s", out)
	}
}

// `target inspect -o json` must keep rendering the bare target: wrapping it
// would break the shape for anything already parsing it.
func TestTargetInspectJSON_KeepsBareTargetShape(t *testing.T) {
	a := profileTestApp(t)
	a.output = formatJSON
	out, err := runCmd(a.targetInspectCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("target inspect -o json: %v", err)
	}
	if strings.Contains(out, `"credentials"`) || strings.Contains(out, `"target":`) {
		t.Errorf("output must stay the bare target, got:\n%s", out)
	}
	if !strings.Contains(out, `"credential_ids"`) {
		t.Errorf("credential_ids is additive and should be present, got:\n%s", out)
	}
}

// The --profile flag must exist and be documented on `target use`, or the
// override is unreachable from the CLI.
func TestTargetUse_HasProfileFlag(t *testing.T) {
	flag := testApp(t).targetUseCmd().Flags().Lookup("profile")
	if flag == nil {
		t.Fatal("`target use` has no --profile flag")
	}
	if flag.Usage == "" {
		t.Error("--profile has no usage text")
	}
}

// A profile that cannot reach the target is rejected, and the error names the
// ones that would have worked rather than leaving the operator to guess.
func TestTargetUse_UnknownProfileNamesTheValidOnes(t *testing.T) {
	_, err := runCmd(profileTestApp(t).targetUseCmd(), "", "eks-prod", "--profile", "nope", "--no-kubeconfig")
	if err == nil {
		t.Fatal("expected an error for an unreachable profile")
	}
	for _, want := range []string{"nope", "ops", "dev"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// The three ways a credential gets chosen must read differently. An unprompted
// default is a guess — kuberoutectl cannot see EKS access entries — so it must
// not be worded like a decision the operator made.
func TestTargetUse_DistinguishesDefaultFromChoice(t *testing.T) {
	a := profileTestApp(t)

	out, err := runCmd(a.targetUseCmd(), "", "eks-prod", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use: %v", err)
	}
	if !strings.Contains(out, "default") || !strings.Contains(out, "--profile") {
		t.Errorf("an unprompted default must say so and point at --profile, got:\n%s", out)
	}

	out, err = runCmd(a.targetUseCmd(), "", "eks-prod", "--profile", "dev", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use --profile: %v", err)
	}
	if !strings.Contains(out, "via dev") {
		t.Errorf("expected the chosen profile, got:\n%s", out)
	}
	if strings.Contains(out, "default") {
		t.Errorf("an explicit choice must not be reported as a default, got:\n%s", out)
	}

	// The choice persists, and the next bare use says it was remembered.
	out, err = runCmd(a.targetUseCmd(), "", "eks-prod", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use (remembered): %v", err)
	}
	if !strings.Contains(out, "via dev") || !strings.Contains(out, "remembered") {
		t.Errorf("expected the remembered profile, got:\n%s", out)
	}
}

// A target with one way in has nothing to report, so the common case is not
// cluttered with an irrelevant "via".
func TestTargetUse_SingleCredentialSaysNothingAboutProfiles(t *testing.T) {
	out, err := runCmd(profileTestApp(t).targetUseCmd(), "", "gke-prod", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use: %v", err)
	}
	if strings.Contains(out, "via") {
		t.Errorf("single-credential target should not mention a profile, got:\n%s", out)
	}
}

func TestCurrent_ReportsProfileAndFlagsAVanishedOne(t *testing.T) {
	a := profileTestApp(t)
	if _, err := runCmd(a.targetUseCmd(), "", "eks-prod", "--profile", "dev", "--no-kubeconfig"); err != nil {
		t.Fatalf("target use: %v", err)
	}
	out, err := runCmd(a.currentCmd(), "")
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if !strings.Contains(out, "Profile") || !strings.Contains(out, "dev") {
		t.Errorf("current should report the profile in use, got:\n%s", out)
	}

	// Simulate a resync that dropped the profile from ~/.aws/config.
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap.Credentials = snap.Credentials[:1] // keep ops, drop dev
	snap.Targets[0].CredentialIDs = []domain.CredentialID{"aws:ops"}
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err = runCmd(a.currentCmd(), "")
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if !strings.Contains(out, "no longer in the cache") {
		t.Errorf("current must flag a profile the cache no longer holds, got:\n%s", out)
	}
}

// `target use -o json` must keep rendering the bare target. Wrapping it to also
// report the credential would break the shape for anything already parsing it —
// the same rule `target inspect` follows. Which credential was used is
// available from `current -o json`.
func TestTargetUseJSON_KeepsBareTargetShape(t *testing.T) {
	a := profileTestApp(t)
	a.output = formatJSON
	out, err := runCmd(a.targetUseCmd(), "", "eks-prod", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use -o json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	for _, forbidden := range []string{"Target", "Credential", "CredentialSource", "target", "credential_source"} {
		if _, ok := got[forbidden]; ok {
			t.Errorf("key %q must not be at the top level; output should be the bare target:\n%s", forbidden, out)
		}
	}
	if _, ok := got["id"]; !ok {
		t.Errorf("expected the bare target's own fields at the top level, got:\n%s", out)
	}
}

// A remembered profile that a resync removed must be reported. Once the fold
// drops it the target has one access path again and looks exactly like one that
// never had a choice — so without an explicit signal the operator who picked a
// break-glass profile is moved back to the default with no output at all.
func TestTargetUse_ReportsAProfileThatVanished(t *testing.T) {
	a := profileTestApp(t)
	if _, err := runCmd(a.targetUseCmd(), "", "eks-prod", "--profile", "dev", "--no-kubeconfig"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap.Credentials = snap.Credentials[:1]                          // dev is gone from ~/.aws/config
	snap.Targets[0].CredentialIDs = []domain.CredentialID{"aws:ops"} // and from the fold
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := runCmd(a.targetUseCmd(), "", "eks-prod", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("target use: %v", err)
	}
	if !strings.Contains(out, "dev") {
		t.Errorf("must name the profile that disappeared, got:\n%s", out)
	}
	if !strings.Contains(out, "ops") {
		t.Errorf("must name the profile now in use, got:\n%s", out)
	}
}
