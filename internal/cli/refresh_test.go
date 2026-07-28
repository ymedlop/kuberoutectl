package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// checkRecordingProvider answers a canned live check and counts the asks.
type checkRecordingProvider struct {
	stubProvider
	res   providers.AccessCheck
	calls int
}

func (c *checkRecordingProvider) CheckAccess(context.Context, domain.Target, []domain.Credential) (providers.AccessCheck, error) {
	c.calls++
	return c.res, nil
}

func refreshApp(t *testing.T, prov *checkRecordingProvider) *app {
	t.Helper()
	a := profileTestApp(t)
	if err := a.registry.Register(prov); err != nil {
		t.Fatalf("register: %v", err)
	}
	snap, err := a.store.LoadSnapshot()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap.Targets[0].ProviderID = "aws"
	snap.Targets[0].AccessCheck = domain.AccessCheckAPI
	snap.Targets[0].OperableCredentialIDs = []domain.CredentialID{"aws:dev"}
	if err := a.store.SaveSnapshot(snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	return a
}

func newCheckProvider(operable ...domain.CredentialID) *checkRecordingProvider {
	return &checkRecordingProvider{
		stubProvider: stubProvider{id: "aws"},
		res:          providers.AccessCheck{Mode: domain.AccessCheckAPI, Operable: operable},
	}
}

// The default must touch nothing. With the cache complete after the widened
// sync, an unasked-for check is pure cost.
func TestTargetInspect_NoCheckWithoutRefresh(t *testing.T) {
	prov := newCheckProvider("aws:ops")
	if _, err := runCmd(refreshApp(t, prov).targetInspectCmd(), "", "eks-prod"); err != nil {
		t.Fatalf("target inspect: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("made %d live checks without --refresh, want 0", prov.calls)
	}
}

// With the flag, the live answer replaces the cached one — including when it
// contradicts it, which is why anyone would ask again.
func TestTargetInspect_RefreshOverridesTheCache(t *testing.T) {
	prov := newCheckProvider("aws:ops") // the cache says dev
	out, err := runCmd(refreshApp(t, prov).targetInspectCmd(), "", "eks-prod", "--refresh")
	if err != nil {
		t.Fatalf("target inspect --refresh: %v", err)
	}
	if prov.calls != 1 {
		t.Fatalf("made %d checks, want exactly 1 — one call answers for every profile", prov.calls)
	}
	ops := profileLine(t, out, "ops")
	if !strings.Contains(ops, string(domain.AccessOperable)) || strings.Contains(ops, string(domain.AccessNotOperable)) {
		t.Errorf("ops should read operable after the refresh: %q", ops)
	}
	if dev := profileLine(t, out, "dev"); !strings.Contains(dev, string(domain.AccessNotOperable)) {
		t.Errorf("dev should read not operable after the refresh: %q", dev)
	}
}

func TestTargetUse_NoCheckWithoutRefresh(t *testing.T) {
	prov := newCheckProvider("aws:ops")
	if _, err := runCmd(refreshApp(t, prov).targetUseCmd(), "", "eks-prod", "--no-kubeconfig"); err != nil {
		t.Fatalf("target use: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("made %d live checks without --refresh, want 0", prov.calls)
	}
}

// Both directions, on stderr, and only under the flag: the operator asked
// whether they can operate here, and silence answers half of that.
func TestTargetUse_RefreshReportsAnAdmission(t *testing.T) {
	prov := newCheckProvider("aws:ops")
	_, stderr, err := runCmdSplit(refreshApp(t, prov).targetUseCmd(), "eks-prod", "--profile", "ops", "--no-kubeconfig", "--refresh")
	if err != nil {
		t.Fatalf("target use --refresh: %v", err)
	}
	if !strings.Contains(stderr, "holds an access entry") {
		t.Errorf("a confirmed admission must be reported, got:\n%s", stderr)
	}
}

// The mine this guarded: get_target used to call TargetService.Resolve, which
// `target label add`, `remove` and `list` also use. Teaching Resolve to check
// access would have given all three a cloud call nothing asked for — and the
// natural implementation is exactly that, since Resolve is the method the
// handler already called.
func TestTargetLabelCommands_NeverCheckAccess(t *testing.T) {
	prov := newCheckProvider("aws:ops")
	a := refreshApp(t, prov)

	if _, err := runCmd(a.targetLabelCmd(), "", "add", "eks-prod", "env=prod"); err != nil {
		t.Fatalf("label add: %v", err)
	}
	if _, err := runCmd(a.targetLabelCmd(), "", "list", "eks-prod"); err != nil {
		t.Fatalf("label list: %v", err)
	}
	if _, err := runCmd(a.targetLabelCmd(), "", "remove", "eks-prod", "env"); err != nil {
		t.Fatalf("label remove: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("the label commands made %d access checks, want 0", prov.calls)
	}
}

// Nor may listing a fleet, which would multiply the cost by the number of
// targets on every display.
func TestTargetList_NeverChecksAccess(t *testing.T) {
	prov := newCheckProvider("aws:ops")
	if _, err := runCmd(refreshApp(t, prov).targetListCmd(), ""); err != nil {
		t.Fatalf("target list: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("target list made %d access checks, want 0", prov.calls)
	}
}
