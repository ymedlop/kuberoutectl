package services

import (
	"context"
	"errors"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

type stubProvider struct{ id domain.ProviderID }

func (s stubProvider) ID() domain.ProviderID             { return s.id }
func (s stubProvider) Capabilities() domain.Capabilities { return domain.Capabilities{} }
func (s stubProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return providers.DiscoveryResult{}, nil
}
func (s stubProvider) Renew(context.Context, domain.Credential) error { return nil }

// fakeResolver resolves only names present in its found map.
type fakeResolver struct{ found map[string]string }

func (f fakeResolver) Resolve(name string) (string, error) {
	if p, ok := f.found[name]; ok {
		return p, nil
	}
	return "", errors.New("not found: " + name)
}

func TestDoctor_ReportsResolvableAndMissing(t *testing.T) {
	reg := providers.NewRegistry()
	_ = reg.Register(stubProvider{id: "azure"})
	_ = reg.Register(stubProvider{id: "aws"})

	resolver := fakeResolver{found: map[string]string{"az": "/usr/bin/az"}}
	d := NewDoctorService(reg, resolver, map[string]string{"azure": "az", "aws": "aws"})

	checks := d.Run()
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
	// List() is sorted: aws first, azure second.
	if checks[0].Name != "provider:aws" || checks[0].Status != CheckFail {
		t.Errorf("aws should fail (aws CLI missing): %+v", checks[0])
	}
	if checks[1].Name != "provider:azure" || checks[1].Status != CheckOK {
		t.Errorf("azure should pass (az resolved): %+v", checks[1])
	}
}

func TestDoctor_ProviderWithoutRequiredBinaryIsOK(t *testing.T) {
	reg := providers.NewRegistry()
	_ = reg.Register(stubProvider{id: "kubeconfig"})
	d := NewDoctorService(reg, fakeResolver{found: map[string]string{}}, map[string]string{})
	checks := d.Run()
	if len(checks) != 1 || checks[0].Status != CheckOK {
		t.Fatalf("provider with no required CLI should be OK: %+v", checks)
	}
}
