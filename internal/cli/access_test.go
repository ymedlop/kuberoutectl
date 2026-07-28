package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// runCmdSplit keeps the two streams apart. The access warning is specified to
// go to stderr so it cannot corrupt piped stdout, and a helper that merges them
// would pass either way.
func runCmdSplit(cmd *cobra.Command, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// accessTestApp is profileTestApp with an access-entry verdict on the
// multi-credential target.
func accessTestApp(t *testing.T, check domain.AccessCheckMode, operable ...domain.CredentialID) *app {
	t.Helper()
	a := profileTestApp(t)
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap.Targets[0].AccessCheck = check
	snap.Targets[0].OperableCredentialIDs = operable
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	return a
}

// The column follows the same rule as PROFILES — worth showing only when some
// target has a choice — and names the profiles confirmed to work.
func TestTargetList_OperableColumn(t *testing.T) {
	out, err := runCmd(accessTestApp(t, domain.AccessCheckAPI, "aws:ops").targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if !strings.Contains(out, "OPERABLE") {
		t.Errorf("expected an OPERABLE column, got:\n%s", out)
	}

	out, err = runCmd(singleCredentialApp(t).targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if strings.Contains(out, "OPERABLE") {
		t.Errorf("OPERABLE must be absent when no target has a choice, got:\n%s", out)
	}
}

// The three cell values, with the emphasis on never rendering blank: a blank
// cell reads as "no", and "we did not check" is not "no".
func TestTargetList_OperableCellValues(t *testing.T) {
	cases := []struct {
		name     string
		check    domain.AccessCheckMode
		operable []domain.CredentialID
		want     string
	}{
		{"confirmed admitted profiles are named", domain.AccessCheckAPI, []domain.CredentialID{"aws:ops"}, "ops"},
		{"all confirmed refused", domain.AccessCheckAPI, nil, "none"},
		{"absence proves nothing", domain.AccessCheckAPIAndConfigMap, nil, "unknown"},
		{"entries do not apply", domain.AccessCheckConfigMap, nil, "unknown"},
		{"the check could not run", domain.AccessCheckUnavailable, nil, "unknown"},
		{"never checked", "", nil, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCmd(accessTestApp(t, tc.check, tc.operable...).targetListCmd(), "")
			if err != nil {
				t.Fatalf("target list: %v", err)
			}
			row := rowContaining(t, out, "eks-prod")
			if !strings.Contains(row, tc.want) {
				t.Errorf("row %q does not carry %q", row, tc.want)
			}
		})
	}
}

// A single-credential target must still render something in the column rather
// than a blank cell, because the column exists as soon as any target has a
// choice — including rows that were never checked.
func TestTargetList_OperableNeverBlankForUncheckedRows(t *testing.T) {
	out, err := runCmd(accessTestApp(t, domain.AccessCheckAPI, "aws:ops").targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	row := rowContaining(t, out, "gke-prod")
	if !strings.Contains(row, "unknown") {
		t.Errorf("an unchecked row must read unknown, not blank: %q", row)
	}
}

func rowContaining(t *testing.T, out, substr string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("no row containing %q in:\n%s", substr, out)
	return ""
}

// inspect must say both what was established about the cluster and what that
// means per profile. The mode alone is jargon; the verdict alone hides why an
// answer is unknown.
func TestTargetInspect_ShowsAccessCheckAndPerProfileVerdict(t *testing.T) {
	out, err := runCmd(accessTestApp(t, domain.AccessCheckAPI, "aws:ops").targetInspectCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if !strings.Contains(out, "Access check") {
		t.Errorf("expected an Access check line, got:\n%s", out)
	}
	if !strings.Contains(out, "operable") || !strings.Contains(out, "not operable") {
		t.Errorf("expected both verdicts in the per-profile breakdown, got:\n%s", out)
	}
}

// A cluster nobody checked must not grow an Access check line, so the common
// single-profile case stays exactly as it reads today.
func TestTargetInspect_NoAccessLineWhenNothingWasChecked(t *testing.T) {
	out, err := runCmd(profileTestApp(t).targetInspectCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if strings.Contains(out, "Access check") {
		t.Errorf("unchecked target must not claim a check happened, got:\n%s", out)
	}
}

// Under a mode where nothing can be established, the verdict must read unknown
// per profile — never "not operable".
func TestTargetInspect_ConfigMapRendersUnknownPerProfile(t *testing.T) {
	out, err := runCmd(accessTestApp(t, domain.AccessCheckConfigMap).targetInspectCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if strings.Contains(out, "not operable") {
		t.Errorf("CONFIG_MAP must never yield a refusal, got:\n%s", out)
	}
	if !strings.Contains(out, "unknown") {
		t.Errorf("expected unknown per profile, got:\n%s", out)
	}
}

// Choosing a profile the cluster is known to refuse warns on stderr and still
// proceeds — the verdict is from the last sync and may be stale, and going in
// to diagnose exactly this is legitimate.
func TestTargetUse_WarnsOnStderrForARefusedProfile(t *testing.T) {
	a := accessTestApp(t, domain.AccessCheckAPI, "aws:ops")
	stdout, stderr, err := runCmdSplit(a.targetUseCmd(), "eks-prod", "--profile", "dev", "--no-kubeconfig")
	if err != nil {
		t.Fatalf("a confirmed refusal must warn, not block: %v", err)
	}
	if !strings.Contains(stderr, "dev") || !strings.Contains(stderr, "ops") {
		t.Errorf("stderr must name the refused profile and a working one, got:\n%s", stderr)
	}
	if strings.Contains(stdout, "no access entry") {
		t.Errorf("the warning belongs on stderr, not stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Recorded selection") {
		t.Errorf("the selection must still be recorded, got:\n%s", stdout)
	}
}

// The assertion that carries the most weight. Most clusters authenticate
// through aws-auth, where nothing can be established — so warning on unknown
// would fire constantly on no evidence, and an alarm that is usually wrong
// trains people to ignore the one time it is right.
func TestTargetUse_SilentWhenOperabilityIsUnknown(t *testing.T) {
	for _, check := range []domain.AccessCheckMode{
		domain.AccessCheckConfigMap,
		domain.AccessCheckAPIAndConfigMap,
		domain.AccessCheckUnavailable,
		"",
	} {
		a := accessTestApp(t, check)
		_, stderr, err := runCmdSplit(a.targetUseCmd(), "eks-prod", "--profile", "dev", "--no-kubeconfig")
		if err != nil {
			t.Fatalf("check=%q: target use: %v", check, err)
		}
		if strings.Contains(stderr, "no access entry") {
			t.Errorf("check=%q warned on an unknown verdict:\n%s", check, stderr)
		}
	}
}
