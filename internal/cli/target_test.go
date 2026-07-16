package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ymedlop/kuberoutectl/internal/cache/jsonstore"
	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func testApp(t *testing.T, targets ...domain.Target) *app {
	t.Helper()
	dir := t.TempDir()
	a := &app{store: jsonstore.New(dir, dir), output: formatText}
	if err := a.store.SaveSnapshot(domain.InventorySnapshot{Targets: targets}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return a
}

// runCmd executes a standalone subcommand with the given stdin and args,
// capturing combined output.
func runCmd(cmd *cobra.Command, stdin string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func targetCount(t *testing.T, a *app) int {
	t.Helper()
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return len(snap.Targets)
}

func TestTargetDelete_RemovesAndReports(t *testing.T) {
	a := testApp(t,
		domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"},
		domain.Target{ID: "aws:eks-2", ProviderID: "aws", Name: "eks-staging"},
	)
	out, err := runCmd(a.targetDeleteCmd(), "", "eks-prod")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "Deleted target:") || !strings.Contains(out, "eks-prod") {
		t.Errorf("unexpected output: %q", out)
	}
	if n := targetCount(t, a); n != 1 {
		t.Errorf("expected 1 target left, got %d", n)
	}
}

func TestTargetDelete_UnknownRefErrors(t *testing.T) {
	a := testApp(t, domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"})
	if _, err := runCmd(a.targetDeleteCmd(), "", "nope"); err == nil {
		t.Fatal("expected error for unknown ref")
	}
	if n := targetCount(t, a); n != 1 {
		t.Errorf("failed delete must not mutate cache, got %d", n)
	}
}

func TestTargetClear_ConfirmYes(t *testing.T) {
	a := testApp(t,
		domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"},
		domain.Target{ID: "aws:eks-2", ProviderID: "aws", Name: "eks-staging"},
	)
	out, err := runCmd(a.targetClearCmd(), "y\n")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if !strings.Contains(out, "Cleared 2 target(s).") {
		t.Errorf("unexpected output: %q", out)
	}
	if n := targetCount(t, a); n != 0 {
		t.Errorf("targets not cleared, got %d", n)
	}
}

func TestTargetClear_AbortsOnNo(t *testing.T) {
	a := testApp(t, domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"})
	out, err := runCmd(a.targetClearCmd(), "n\n")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("expected abort, got %q", out)
	}
	if n := targetCount(t, a); n != 1 {
		t.Errorf("abort must not clear, got %d", n)
	}
}

func TestTargetClear_AbortsOnEOF(t *testing.T) {
	// Non-interactive: no input, no --yes. Must not delete.
	a := testApp(t, domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"})
	out, err := runCmd(a.targetClearCmd(), "")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("EOF should abort, got %q", out)
	}
	if n := targetCount(t, a); n != 1 {
		t.Errorf("EOF abort must not clear, got %d", n)
	}
}

func TestTargetClear_YesFlagSkipsPrompt(t *testing.T) {
	a := testApp(t, domain.Target{ID: "aws:eks-1", ProviderID: "aws", Name: "eks-prod"})
	out, err := runCmd(a.targetClearCmd(), "", "--yes")
	if err != nil {
		t.Fatalf("clear --yes: %v", err)
	}
	if !strings.Contains(out, "Cleared 1 target(s).") {
		t.Errorf("unexpected output: %q", out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("--yes must not prompt: %q", out)
	}
	if n := targetCount(t, a); n != 0 {
		t.Errorf("--yes should clear, got %d", n)
	}
}

func TestTargetClear_EmptyNoPrompt(t *testing.T) {
	a := testApp(t)
	out, err := runCmd(a.targetClearCmd(), "")
	if err != nil {
		t.Fatalf("clear empty: %v", err)
	}
	if !strings.Contains(out, "No targets to clear.") {
		t.Errorf("unexpected output: %q", out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("empty must not prompt: %q", out)
	}
}
