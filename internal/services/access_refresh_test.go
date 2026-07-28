package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// checkingProvider answers a canned live check and records that it was asked.
type checkingProvider struct {
	credentialActivatableProvider
	res   providers.AccessCheck
	err   error
	calls int
}

func (c *checkingProvider) CheckAccess(context.Context, domain.Target, []domain.Credential) (providers.AccessCheck, error) {
	c.calls++
	return c.res, c.err
}

// neverCheckingProvider fails the test if anything asks it. It is how "no call
// without the flag" is asserted — a result-level check passes just as well when
// the call happened and its answer was thrown away.
type neverCheckingProvider struct {
	credentialActivatableProvider
	t *testing.T
}

func (n *neverCheckingProvider) CheckAccess(context.Context, domain.Target, []domain.Credential) (providers.AccessCheck, error) {
	n.t.Helper()
	n.t.Error("a live access check ran without --refresh")
	return providers.AccessCheck{}, nil
}

func refreshSetup(t *testing.T, prov providers.Provider) *SelectionService {
	t.Helper()
	store := storeWithMultiCredentialTarget()
	store.snap.Targets[0].AccessCheck = domain.AccessCheckAPI
	store.snap.Targets[0].OperableCredentialIDs = []domain.CredentialID{"aws:dev"}
	reg := providers.NewRegistry()
	if err := reg.Register(prov); err != nil {
		t.Fatalf("register: %v", err)
	}
	return NewSelectionService(store, reg, fixedNow)
}

// The default has to make no call at all. This is the assertion the flag exists
// for: with the cache complete after the widened sync, an unasked-for check is
// pure cost.
func TestUseTarget_NoLiveCheckWithoutRefresh(t *testing.T) {
	svc := refreshSetup(t, &neverCheckingProvider{
		credentialActivatableProvider: credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}},
		t:                             t,
	})

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	// And the cached verdict is still reported, so the default is not silent.
	if res.AccessVerdict != domain.AccessNotOperable {
		t.Errorf("AccessVerdict = %q, want the cached %q for the primary",
			res.AccessVerdict, domain.AccessNotOperable)
	}
}

// With the flag, the live answer replaces the cached one — including when it
// contradicts it, which is the entire point of asking again.
func TestUseTarget_RefreshOverridesTheCachedVerdict(t *testing.T) {
	prov := &checkingProvider{
		credentialActivatableProvider: credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}},
		res: providers.AccessCheck{
			Mode:     domain.AccessCheckAPI,
			Operable: []domain.CredentialID{"aws:ops"}, // the cache said dev
		},
	}
	svc := refreshSetup(t, prov)

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, Refresh: true})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("made %d checks, want exactly 1 — one call answers for every credential", prov.calls)
	}
	if res.AccessVerdict != domain.AccessOperable {
		t.Errorf("AccessVerdict = %q, want %q from the live answer", res.AccessVerdict, domain.AccessOperable)
	}
	if res.AccessWarning != "" {
		t.Errorf("no warning is due once the live check admits the profile: %q", res.AccessWarning)
	}
}

// A failed live check must not block the activation. The natural implementation
// propagates the error — every other failure in UseTarget does — and would lock
// the operator out of a cluster at the moment they are diagnosing it.
func TestUseTarget_FailedRefreshDoesNotBlockActivation(t *testing.T) {
	prov := &checkingProvider{
		credentialActivatableProvider: credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}},
		res: providers.AccessCheck{
			Mode:   domain.AccessCheckUnavailable,
			Reason: "profile ops may lack eks:ListAccessEntries on this cluster",
		},
	}
	svc := refreshSetup(t, prov)

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, Refresh: true})
	if err != nil {
		t.Fatalf("a failed check must not block: %v", err)
	}
	if res.AccessVerdict != domain.AccessUnknown {
		t.Errorf("AccessVerdict = %q, want %q — nothing was established", res.AccessVerdict, domain.AccessUnknown)
	}
	if res.AccessReason == "" {
		t.Error("a failed check must say why")
	}
	if res.AccessWarning != "" {
		t.Errorf("an unknown verdict must not warn: %q", res.AccessWarning)
	}
}

// A provider that cannot answer — no such capability, or none registered — must
// leave the cached verdict exactly as it was rather than blanking it.
func TestUseTarget_RefreshOnAProviderThatCannotCheckKeepsTheCache(t *testing.T) {
	svc := refreshSetup(t, &credentialActivatableProvider{
		activatableProvider: activatableProvider{id: "aws", canSwitch: true},
	})

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, Refresh: true})
	if err != nil {
		t.Fatalf("UseTarget: %v", err)
	}
	if res.Target.AccessCheck != domain.AccessCheckAPI {
		t.Errorf("AccessCheck = %q, want the cached %q left intact",
			res.Target.AccessCheck, domain.AccessCheckAPI)
	}
	if res.AccessVerdict != domain.AccessNotOperable {
		t.Errorf("AccessVerdict = %q, want the cached verdict", res.AccessVerdict)
	}
}

// A provider that answers with an error must leave the cached verdict exactly as
// it was. The failure this guards is subtle: the error branch returns a non-zero
// Reason, so a caller gating the override on "was anything attempted" takes it,
// overwrites AccessCheck with the empty Mode and blanks a perfectly good cached
// answer — replacing knowledge with `unknown` because a diagnostic failed.
//
// The scaffolding for this test existed unused for a while: checkingProvider has
// carried an `err` field since it was written, and nothing set it. An untested
// error branch on an optional capability is the one that looks unreachable
// through today's callers and stops being so at the next provider.
func TestUseTarget_ProviderErrorMustNotBlankTheCache(t *testing.T) {
	prov := &checkingProvider{
		credentialActivatableProvider: credentialActivatableProvider{activatableProvider: activatableProvider{id: "aws", canSwitch: true}},
		err:                           errors.New("aws: target belongs to provider \"gcp\""),
	}
	svc := refreshSetup(t, prov)

	res, err := svc.UseTarget(context.Background(), "t1", UseTargetOptions{Activate: true, Refresh: true})
	if err != nil {
		t.Fatalf("a provider error must not fail the command: %v", err)
	}
	if res.Target.AccessCheck != domain.AccessCheckAPI {
		t.Errorf("AccessCheck = %q, want the cached %q left intact", res.Target.AccessCheck, domain.AccessCheckAPI)
	}
	if len(res.Target.OperableCredentialIDs) != 1 {
		t.Errorf("OperableCredentialIDs = %v, want the cached set left intact", res.Target.OperableCredentialIDs)
	}
	if res.AccessReason == "" {
		t.Error("the failure must still be reported")
	}
}
