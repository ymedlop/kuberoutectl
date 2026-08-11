package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

func newAccessHandler(t *testing.T, check domain.AccessCheckMode, operable ...domain.CredentialID) *handler {
	t.Helper()
	snap := multiCredentialSnapshot()
	snap.Targets[0].AccessCheck = check
	snap.Targets[0].OperableCredentialIDs = operable
	prov := &credentialFakeProvider{fakeProvider: fakeProvider{
		id:   "aws",
		caps: domain.Capabilities{CanSwitchContext: true},
	}}
	h, _ := newTestHandlerWithStore(t, snap, prov)
	return h
}

// An agent driving kuberoutectl is the case this whole feature was asked for:
// "find a cluster you can operate and run this analysis". It must not be handed
// a profile the cluster is known to refuse without being told.
func TestMCPUseTarget_ReturnsAccessWarningForARefusedProfile(t *testing.T) {
	h := newAccessHandler(t, domain.AccessCheckAPI, "aws:ops")

	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"})
	if err != nil {
		t.Fatalf("a confirmed refusal must warn, not fail: %v", err)
	}
	if out.AccessWarning == "" {
		t.Fatal("no access_warning for a profile confirmed to hold no access entry")
	}
	if !strings.Contains(out.AccessWarning, "ops") {
		t.Errorf("warning does not name a profile that would work: %q", out.AccessWarning)
	}
}

// The parity that matters is on *silence*: warning whenever operability is
// unknown would fire on most clusters, since CONFIG_MAP is the default for
// anything created outside the console.
func TestMCPUseTarget_SilentWhenOperabilityIsUnknown(t *testing.T) {
	for _, check := range []domain.AccessCheckMode{
		domain.AccessCheckConfigMap,
		domain.AccessCheckAPIAndConfigMap,
		domain.AccessCheckUnavailable,
		"",
	} {
		h := newAccessHandler(t, check)
		_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "dev"})
		if err != nil {
			t.Fatalf("check=%q: useTarget: %v", check, err)
		}
		if out.AccessWarning != "" {
			t.Errorf("check=%q warned on an unknown verdict: %q", check, out.AccessWarning)
		}
	}
}

// get_target renders domain.Target, so the new fields ship with no wiring —
// but "it should just work" is exactly the claim worth a test, since a handler
// that copied fields by hand would silently drop them.
func TestMCPGetTarget_CarriesAccessFields(t *testing.T) {
	h := newAccessHandler(t, domain.AccessCheckAPI, "aws:ops")

	_, out, err := h.getTarget(context.Background(), nil, GetTargetInput{Ref: "eks-prod"})
	if err != nil {
		t.Fatalf("getTarget: %v", err)
	}
	if out.Target.AccessCheck != domain.AccessCheckAPI {
		t.Errorf("AccessCheck = %q, want %q", out.Target.AccessCheck, domain.AccessCheckAPI)
	}
	if got := out.Target.CredentialAccess("aws:dev"); got != domain.AccessNotOperable {
		t.Errorf("CredentialAccess(aws:dev) = %q, want %q", got, domain.AccessNotOperable)
	}
}

// checkingFakeProvider answers a canned live check and counts the asks.
type checkingFakeProvider struct {
	credentialFakeProvider
	res   providers.AccessCheck
	calls int
}

func (c *checkingFakeProvider) CheckAccess(context.Context, domain.Target, []domain.Credential) (providers.AccessCheck, error) {
	c.calls++
	return c.res, nil
}

func refreshHandler(t *testing.T, operable ...domain.CredentialID) (*handler, *checkingFakeProvider) {
	t.Helper()
	snap := multiCredentialSnapshot()
	snap.Targets[0].AccessCheck = domain.AccessCheckAPI
	snap.Targets[0].OperableCredentialIDs = []domain.CredentialID{"aws:dev"}
	prov := &checkingFakeProvider{
		credentialFakeProvider: credentialFakeProvider{fakeProvider: fakeProvider{
			id:   "aws",
			caps: domain.Capabilities{CanSwitchContext: true},
		}},
		res: providers.AccessCheck{Mode: domain.AccessCheckAPI, Operable: operable},
	}
	h, _ := newTestHandlerWithStore(t, snap, prov)
	return h, prov
}

// Same name, same default, same meaning as the CLI's --refresh: an agent and a
// human are never told different things about the same cluster, and neither
// pays for a call nobody asked for.
func TestMCPUseTarget_RefreshDefaultsOff(t *testing.T) {
	h, prov := refreshHandler(t, "aws:ops")

	if _, _, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true}); err != nil {
		t.Fatalf("useTarget: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("made %d live checks without refresh, want 0", prov.calls)
	}
}

func TestMCPUseTarget_RefreshReportsTheVerdictBothWays(t *testing.T) {
	h, prov := refreshHandler(t, "aws:ops")

	_, out, err := h.useTarget(context.Background(), nil, UseTargetInput{Ref: "eks-prod", Activate: true, Profile: "ops", Refresh: true})
	if err != nil {
		t.Fatalf("useTarget: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("made %d checks, want 1", prov.calls)
	}
	if out.AccessVerdict != string(domain.AccessOperable) {
		t.Errorf("access_verdict = %q, want %q — the positive case is what an agent picks on",
			out.AccessVerdict, domain.AccessOperable)
	}
	if out.AccessWarning != "" {
		t.Errorf("no warning is due for an admitted profile: %q", out.AccessWarning)
	}
}

// get_target defaults off too, and refreshing it must not disturb anything else
// it renders.
func TestMCPGetTarget_RefreshDefaultsOff(t *testing.T) {
	h, prov := refreshHandler(t, "aws:ops")

	_, out, err := h.getTarget(context.Background(), nil, GetTargetInput{Ref: "eks-prod"})
	if err != nil {
		t.Fatalf("getTarget: %v", err)
	}
	if prov.calls != 0 {
		t.Errorf("made %d live checks without refresh, want 0", prov.calls)
	}
	if out.Target.Name != "eks-prod" {
		t.Errorf("target = %+v, want it rendered as before", out.Target)
	}

	_, out, err = h.getTarget(context.Background(), nil, GetTargetInput{Ref: "eks-prod", Refresh: true})
	if err != nil {
		t.Fatalf("getTarget --refresh: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("made %d checks with refresh, want 1", prov.calls)
	}
	if got := out.Target.CredentialAccess("aws:ops"); got != domain.AccessOperable {
		t.Errorf("ops = %q, want the live %q", got, domain.AccessOperable)
	}
}
