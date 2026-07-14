package aws

import (
	"context"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
)

func TestActivate_RunsUpdateKubeconfig(t *testing.T) {
	runner := execx.NewFakeRunner()
	key := "aws eks update-kubeconfig --name eks-prod-frankfurt --region eu-central-1 --profile prod-sso"
	runner.Responses[key] = execx.FakeResponse{}
	p := New(fakeResolver{path: "aws"}, runner)

	target := domain.Target{
		ID:         "t1",
		ProviderID: ProviderID,
		Name:       "eks-prod-frankfurt",
		Region:     "eu-central-1",
		Metadata:   map[string]string{"profile": "prod-sso"},
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
		t.Errorf("expected `aws eks update-kubeconfig ...` to be invoked, calls=%v", runner.Calls)
	}
}

func TestActivate_MissingRegionErrors(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, execx.NewFakeRunner())
	err := p.Activate(context.Background(), domain.Target{ID: "t1", Name: "eks"})
	if err == nil {
		t.Fatal("expected error when region is missing")
	}
}
