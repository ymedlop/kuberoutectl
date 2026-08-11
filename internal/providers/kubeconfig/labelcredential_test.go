package kubeconfig

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/providers/providertest"
)

// D2: the credential label is an invariant every adapter shares, so the
// assertion lives in providertest and only the fixture differs here.
func TestDiscover_EveryTargetLabelsItsCredential(t *testing.T) {
	runner := execx.NewFakeRunner()
	runner.Responses["kubectl config view --raw -o json"] = execx.FakeResponse{Stdout: readFixture(t, "config-view.json")}
	res, err := New(fakeResolver{path: "kubectl"}, runner).Discover(context.Background(), providers.DiscoveryInput{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	providertest.AssertEveryTargetLabelsItsCredential(t, res)
}
