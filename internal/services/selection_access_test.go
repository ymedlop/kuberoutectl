package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// accessCheckedSetup is the multi-credential fixture with an access-entry
// verdict on it: mode `api`, only `ops` admitted. Under that mode `dev` is a
// confirmed refusal, not an absence of information.
func accessCheckedSetup(check domain.AccessCheckMode, operable ...domain.CredentialID) (*memStore, *SelectionService) {
	store := storeWithMultiCredentialTarget()
	store.snap.Targets[0].AccessCheck = check
	store.snap.Targets[0].OperableCredentialIDs = operable
	prov := &credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}}
	reg := providers.NewRegistry()
	_ = reg.Register(prov)
	return store, NewSelectionService(store, reg, fixedNow)
}

// Choosing a profile the cluster is known to refuse must warn — and must still
// go through. The verdict is cached and may be stale, and entering a cluster to
// diagnose exactly this is legitimate, so this reports rather than blocks.
func TestUseTarget_WarnsWhenChosenCredentialIsConfirmedRefused(t *testing.T) {
	_, svc := accessCheckedSetup(domain.AccessCheckAPI, "aws:ops")

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"})
	if err != nil {
		t.Fatalf("a confirmed refusal must warn, not block: %v", err)
	}
	if res.AccessWarning == "" {
		t.Fatal("no AccessWarning for a credential confirmed to hold no access entry")
	}
	if !strings.Contains(res.AccessWarning, "dev") {
		t.Errorf("warning does not name the chosen profile: %q", res.AccessWarning)
	}
	if !strings.Contains(res.AccessWarning, "ops") {
		t.Errorf("warning does not name a profile that would work: %q", res.AccessWarning)
	}
}

// The assertion that matters most: `unknown` must produce no warning at all.
// Most clusters are CONFIG_MAP, so warning on unknown would fire constantly on
// nothing, and an alarm that is usually wrong trains people to ignore the one
// time it is right.
func TestUseTarget_SilentWhenOperabilityIsUnknown(t *testing.T) {
	for _, check := range []domain.AccessCheckMode{
		domain.AccessCheckConfigMap,
		domain.AccessCheckAPIAndConfigMap,
		domain.AccessCheckUnavailable,
		"", // never checked
	} {
		_, svc := accessCheckedSetup(check)
		res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"})
		if err != nil {
			t.Fatalf("check=%q: UseTarget: %v", check, err)
		}
		if res.AccessWarning != "" {
			t.Errorf("check=%q produced a warning on an unknown verdict: %q", check, res.AccessWarning)
		}
	}
}

// The admitted profile must not be warned about.
func TestUseTarget_NoWarningForAnAdmittedCredential(t *testing.T) {
	_, svc := accessCheckedSetup(domain.AccessCheckAPI, "aws:ops")

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "ops"})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.AccessWarning != "" {
		t.Errorf("warned about the admitted profile: %q", res.AccessWarning)
	}
}

// Edge case 12: when every profile is confirmed refused there is no alternative
// to name. The warning must still fire — and must not invent one.
func TestUseTarget_WarnsWithNoAlternativeWhenAllAreRefused(t *testing.T) {
	_, svc := accessCheckedSetup(domain.AccessCheckAPI)

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, CredentialName: "dev"})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.AccessWarning == "" {
		t.Fatal("no warning when every profile is refused")
	}
	if strings.Contains(res.AccessWarning, "ops") {
		t.Errorf("warning names ops as a working alternative, but it is refused too: %q", res.AccessWarning)
	}
}
