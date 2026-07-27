package azure

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/providers/providertest"
)

// D2: the credential label is an invariant every adapter shares, so the
// assertion lives in providertest and only the fixture differs here.
func TestDiscover_EveryTargetLabelsItsCredential(t *testing.T) {
	res, err := newFakeAzProvider(t).Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	providertest.AssertEveryTargetLabelsItsCredential(t, res)
}
