package aws

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// LabelCredential must resolve to a real credential in the same snapshot, in
// every provider. D2 in the plan: a system label one adapter leaves unset makes
// `--selector kuberoutectl.io/credential=x` answer "no match" for that
// provider's targets when the truth is "not implemented" — indistinguishable to
// the operator, so the label is only useful if all four populate it.
func TestDiscover_EveryTargetLabelsItsCredential(t *testing.T) {
	p, _ := newFakeAWSProvider(t)
	res, err := p.Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	byID := map[domain.CredentialID]string{}
	for _, c := range res.Credentials {
		byID[c.ID] = c.Name
	}
	if len(res.Targets) == 0 {
		t.Fatal("fixture yielded no targets; this test would vacuously pass")
	}
	for _, tg := range res.Targets {
		got := tg.SystemLabels[domain.LabelCredential]
		if got == "" {
			t.Errorf("target %q has no %s label", tg.ID, domain.LabelCredential)
			continue
		}
		want, ok := byID[tg.CredentialID]
		if !ok {
			t.Errorf("target %q names credential %q, absent from the snapshot", tg.ID, tg.CredentialID)
			continue
		}
		if got != want {
			t.Errorf("target %q labels credential %q, but %q is named %q", tg.ID, got, tg.CredentialID, want)
		}
	}
}
