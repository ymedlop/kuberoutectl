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

// The full matrix of what `target use` says about access, cached and refreshed.
//
// The rule under test is that the flag changes *freshness*, not whether we
// speak. Before this, a positive and an inconclusive answer were both silent
// without --refresh, so silence meant four different things and could not be
// read as any of them.
func TestTargetUse_AccessIsReportedCachedAndRefreshed(t *testing.T) {
	cases := []struct {
		name      string
		mode      domain.AccessCheckMode
		reason    string
		operable  []domain.CredentialID
		profile   string
		refresh   bool
		wantIn    []string
		wantNotIn []string
		wantEmpty bool
	}{
		{
			name: "cached admission is reported as history",
			mode: domain.AccessCheckAPI, operable: []domain.CredentialID{"aws:ops"}, profile: "ops",
			wantIn: []string{"ops", "held an access entry", "at the last sync"},
		},
		{
			name: "refreshed admission drops the sync clause",
			mode: domain.AccessCheckAPI, operable: []domain.CredentialID{"aws:ops"}, profile: "ops", refresh: true,
			wantIn:    []string{"ops", "holds an access entry"},
			wantNotIn: []string{"at the last sync"},
		},
		{
			name: "cached refusal warns, in the past tense",
			mode: domain.AccessCheckAPI, operable: []domain.CredentialID{"aws:ops"}, profile: "dev",
			wantIn: []string{"Warning", "dev", "had no access entry", "at the last sync", "ops"},
		},
		{
			name: "refreshed refusal warns without it",
			mode: domain.AccessCheckAPI, operable: []domain.CredentialID{"aws:ops"}, profile: "dev", refresh: true,
			wantIn:    []string{"Warning", "dev", "has no access entry"},
			wantNotIn: []string{"at the last sync"},
		},
		{
			// The case that prompted this: their fleet's mode, a profile that is
			// not admitted, and previously nothing at all on stderr.
			name: "refreshed inconclusive explains why, and does not read as a refusal",
			mode: domain.AccessCheckAPIAndConfigMap, operable: []domain.CredentialID{"aws:ops"}, profile: "dev", refresh: true,
			wantIn: []string{"Could not tell", "dev", "aws-auth"},
		},
		{
			name: "refreshed inconclusive under config_map names the mode",
			mode: domain.AccessCheckConfigMap, profile: "dev", refresh: true,
			wantIn: []string{"Could not tell", "CONFIG_MAP"},
		},
		{
			// Mode AND Reason together — the shape the real AWS provider emits
			// when the entry list could not be fetched. The earlier fixtures never
			// produced it, so the branch that handles it went untested while a
			// CONFIG_MAP case that production never emits was asserted instead.
			name: "a refresh that could not run keeps the cached answer visible",
			mode: domain.AccessCheckUnavailable, reason: "profile ops may lack eks:ListAccessEntries on this cluster",
			operable: []domain.CredentialID{"aws:ops"}, profile: "ops", refresh: true,
			wantIn: []string{"Could not check access entries", "eks:ListAccessEntries"},
		},
		{
			// Silence keeps exactly one meaning: nothing was established, and you
			// did not ask. Explaining it on every use would print a line on most
			// clusters in a real fleet, and a message that always appears stops
			// being read.
			name: "cached inconclusive stays silent",
			mode: domain.AccessCheckAPIAndConfigMap, operable: []domain.CredentialID{"aws:ops"}, profile: "dev",
			wantEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := &checkRecordingProvider{
				stubProvider: stubProvider{id: "aws"},
				res:          providers.AccessCheck{Mode: tc.mode, Operable: tc.operable, Reason: tc.reason},
			}
			a := refreshApp(t, prov)
			// refreshApp seeds api/aws:dev; override with this case's fixture.
			snap, err := a.store.LoadSnapshot()
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			snap.Targets[0].AccessCheck, snap.Targets[0].OperableCredentialIDs = tc.mode, tc.operable
			if err := a.store.SaveSnapshot(snap); err != nil {
				t.Fatalf("save: %v", err)
			}

			args := []string{"eks-prod", "--profile", tc.profile, "--no-kubeconfig"}
			if tc.refresh {
				args = append(args, "--refresh")
			}
			_, stderr, err := runCmdSplit(a.targetUseCmd(), args...)
			if err != nil {
				t.Fatalf("target use: %v", err)
			}

			if tc.wantEmpty {
				if strings.TrimSpace(stderr) != "" {
					t.Errorf("expected silence, got:\n%s", stderr)
				}
				return
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q missing %q", strings.TrimSpace(stderr), want)
				}
			}
			for _, unwanted := range tc.wantNotIn {
				if strings.Contains(stderr, unwanted) {
					t.Errorf("stderr %q must not contain %q", strings.TrimSpace(stderr), unwanted)
				}
			}
			// Nothing inconclusive may read as a refusal.
			if strings.Contains(stderr, "Could not tell") && strings.Contains(stderr, "Warning") {
				t.Errorf("an inconclusive answer must not be phrased as a warning: %q", stderr)
			}
		})
	}
}

// A provider with no access-entry concept must stay silent even under --refresh:
// there is no question to answer, and explaining the absence of one would invent
// a subject.
func TestTargetUse_RefreshOnAProviderWithNoAccessConceptIsSilent(t *testing.T) {
	if got := describeAccessUnknown("me", ""); got != "" {
		t.Errorf("want silence for a target with no mode, got %q", got)
	}
	if got := describeAccessUnknown("me", domain.AccessCheckUnavailable); got != "" {
		t.Errorf("an unavailable check reports through AccessReason, not here; got %q", got)
	}
}
