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
	// Per profile, not merely "both words appear somewhere": a breakdown that
	// attached the verdicts to the wrong profiles would satisfy an unpaired
	// existence check while telling the operator the exact opposite of the truth.
	ops := profileLine(t, out, "ops")
	if !strings.Contains(ops, string(domain.AccessOperable)) || strings.Contains(ops, string(domain.AccessNotOperable)) {
		t.Errorf("ops holds the access entry; its line reads %q", ops)
	}
	if dev := profileLine(t, out, "dev"); !strings.Contains(dev, string(domain.AccessNotOperable)) {
		t.Errorf("dev is absent from the list under api mode; its line reads %q", dev)
	}
}

// profileLine returns the `profile <name> …` line from `target inspect`.
func profileLine(t *testing.T, out, name string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "profile" && fields[1] == name {
			return line
		}
	}
	t.Fatalf("no profile line for %q in:\n%s", name, out)
	return ""
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

// columnValue reads one cell out of the tab-written listing, by column name.
//
// Asserting `strings.Contains(row, want)` instead is unreliable and was an
// actual bug in this file: a value can appear in a neighbouring cell, so the
// assertion passes regardless of the column under test. A cell assertion has to
// name the cell.
func columnValue(t *testing.T, out, rowSubstr, column string) string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("no output to read a column from")
	}
	header := strings.Fields(lines[0])
	idx := -1
	for i, h := range header {
		if h == column {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("no %s column in header %q", column, lines[0])
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, rowSubstr) {
			continue
		}
		cells := strings.Fields(line)
		// Positional indexing only holds while every cell is non-empty. Rather
		// than silently reading the wrong column when that stops being true, fail
		// and say so.
		if len(cells) != len(header) {
			t.Fatalf("row %q has %d cells but the header has %d; this helper cannot index it safely",
				line, len(cells), len(header))
		}
		return cells[idx]
	}
	t.Fatalf("no row containing %q in:\n%s", rowSubstr, out)
	return ""
}

// The server version is the first thing you look at when choosing a cluster, and
// it was only reachable through `inspect` one target at a time. It costs nothing
// to show: it is already persisted on the target by discovery.
func TestTargetList_ShowsKubernetesVersion(t *testing.T) {
	a := profileTestApp(t)
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap.Targets[0].KubernetesVersion = "1.29"
	snap.Targets[1].KubernetesVersion = "" // never discovered, e.g. a kubeconfig context
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := runCmd(a.targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if got := columnValue(t, out, "eks-prod", "VERSION"); got != "1.29" {
		t.Errorf("VERSION = %q, want %q\n%s", got, "1.29", out)
	}
	// Never blank: an empty cell in a table reads as a value, and "we do not know
	// this cluster's version" is not the same as an empty version.
	if got := columnValue(t, out, "gke-prod", "VERSION"); got != domain.VersionUnknown {
		t.Errorf("an unknown version must render as %q, got %q\n%s", domain.VersionUnknown, got, out)
	}
}

// The operability verdict moves out of the listing entirely. In a fleet where
// clusters are reached by one profile each — the common case — the check never
// runs, so the column said `unknown` on every row: table width spent training
// the reader to ignore it. The verdict stays in `target inspect`, where it is
// asked for.
func TestTargetList_HasNoOperableColumn(t *testing.T) {
	out, err := runCmd(accessTestApp(t, domain.AccessCheckAPI, "aws:ops").targetListCmd(), "")
	if err != nil {
		t.Fatalf("target list: %v", err)
	}
	if strings.Contains(out, "OPERABLE") {
		t.Errorf("the OPERABLE column should be gone, got:\n%s", out)
	}
	if !strings.Contains(out, "PROFILES") {
		t.Errorf("PROFILES must survive — it is what tells you a choice exists:\n%s", out)
	}
}
