package azure

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
)

func TestActivate_RunsGetCredentials(t *testing.T) {
	runner := execx.NewFakeRunner()
	key := "az aks get-credentials --subscription sub-1 --resource-group rg-platform --name aks-prod-weu --overwrite-existing"
	runner.Responses[key] = execx.FakeResponse{}
	p := New(fakeResolver{path: "az"}, runner)

	target := domain.Target{
		ID:         "t1",
		ProviderID: ProviderID,
		ScopeID:    "sub-1",
		Name:       "aks-prod-weu",
		Metadata:   map[string]string{"resource_group": "rg-platform"},
	}
	if err := p.Activate(context.Background(), target); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	found := false
	for _, c := range runner.Calls {
		if c == key {
			found = true
		}
	}
	if !found {
		t.Errorf("expected `az aks get-credentials ...` to be invoked, calls=%v", runner.Calls)
	}
}

func TestActivate_MissingResourceGroupErrors(t *testing.T) {
	p := New(fakeResolver{path: "az"}, execx.NewFakeRunner())
	err := p.Activate(context.Background(), domain.Target{ID: "t1", Name: "aks", ScopeID: "sub-1"})
	if err == nil {
		t.Fatal("expected error when resource_group metadata is missing")
	}
}
