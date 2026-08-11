// Package providertest holds assertions that every provider adapter must
// satisfy identically. It exists so a cross-cutting invariant is written once
// and checked in every package, instead of being copy-pasted per adapter and
// drifting.
//
// It is imported only from _test files, so nothing here reaches the shipped
// binary despite the testing dependency — the same arrangement as net/http/httptest.
package providertest

import (
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// AssertEveryTargetLabelsItsCredential checks that each discovered target
// carries kuberoutectl.io/credential and that the value names a credential
// present in the same result.
//
// Every adapter must set this label, not just the ones where several
// credentials can reach a target. A label one adapter leaves unset makes
// `--selector kuberoutectl.io/credential=x` answer "no match" for that
// provider's targets when the truth is "not implemented" — and the operator
// cannot tell those two apart from the output.
//
// It fails rather than skips on an empty result: a fixture that discovers
// nothing would otherwise make this pass without checking anything.
func AssertEveryTargetLabelsItsCredential(t *testing.T, res providers.DiscoveryResult) {
	t.Helper()

	if len(res.Targets) == 0 {
		t.Fatal("fixture yielded no targets; this assertion would vacuously pass")
	}
	nameByID := make(map[domain.CredentialID]string, len(res.Credentials))
	for _, c := range res.Credentials {
		nameByID[c.ID] = c.Name
	}
	for _, tg := range res.Targets {
		got := tg.SystemLabels[domain.LabelCredential]
		if got == "" {
			t.Errorf("target %q has no %s label", tg.ID, domain.LabelCredential)
			continue
		}
		want, ok := nameByID[tg.CredentialID]
		if !ok {
			t.Errorf("target %q names credential %q, which is absent from the result", tg.ID, tg.CredentialID)
			continue
		}
		if got != want {
			t.Errorf("target %q labels credential %q, but %q is named %q", tg.ID, got, tg.CredentialID, want)
		}
	}
}
