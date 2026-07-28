package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
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
