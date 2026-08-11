package aws

import (
	"context"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// The AWS adapter must satisfy the optional CredentialActivator interface, or
// the selection service silently falls back to the primary and --profile stops
// working with no error anywhere.
func TestProviderImplementsCredentialActivator(t *testing.T) {
	var _ providers.CredentialActivator = (*Provider)(nil)
}

func multiProfileTarget() domain.Target {
	return domain.Target{
		ID:            "arn:aws:eks:eu-central-1:1234:cluster/eks-prod-frankfurt",
		ProviderID:    ProviderID,
		Name:          "eks-prod-frankfurt",
		Region:        "eu-central-1",
		CredentialID:  credentialID("prod-sso"),
		CredentialIDs: []domain.CredentialID{credentialID("prod-sso"), credentialID("ops")},
		Metadata:      map[string]string{"profile": "prod-sso"},
	}
}

// ActivateAs must drive the chosen credential's profile, not the target's
// recorded primary — that is the entire point of the override.
func TestActivateAs_UsesChosenProfileNotThePrimary(t *testing.T) {
	runner := execx.NewFakeRunner()
	want := "aws eks update-kubeconfig --name eks-prod-frankfurt --region eu-central-1 --profile ops"
	runner.Responses[want] = execx.FakeResponse{}
	p := New(fakeResolver{path: "aws"}, runner)

	cred := domain.Credential{ID: credentialID("ops"), Name: "ops", Metadata: map[string]string{"profile": "ops"}}
	if err := p.ActivateAs(context.Background(), multiProfileTarget(), cred); err != nil {
		t.Fatalf("ActivateAs: %v", err)
	}
	for _, c := range runner.Calls {
		if c == want {
			return
		}
	}
	t.Errorf("expected %q, calls=%v", want, runner.Calls)
}

// A credential with no profile metadata cannot be turned into an aws
// invocation. Failing loudly beats falling back to the primary, which would
// silently put the operator on a different identity than they asked for.
func TestActivateAs_RejectsCredentialWithoutProfile(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, execx.NewFakeRunner())
	cred := domain.Credential{ID: "aws:ops", Name: "ops"}

	err := p.ActivateAs(context.Background(), multiProfileTarget(), cred)
	if err == nil {
		t.Fatal("expected an error for a credential carrying no profile")
	}
	if !strings.Contains(err.Error(), "ops") {
		t.Errorf("error %q should name the credential it could not use", err)
	}
}

// ActivateAs keeps Activate's own guard: without a region there is no valid
// `aws eks update-kubeconfig` to run.
func TestActivateAs_MissingRegionErrors(t *testing.T) {
	p := New(fakeResolver{path: "aws"}, execx.NewFakeRunner())
	target := multiProfileTarget()
	target.Region = ""
	cred := domain.Credential{ID: credentialID("ops"), Name: "ops", Metadata: map[string]string{"profile": "ops"}}

	if err := p.ActivateAs(context.Background(), target, cred); err == nil {
		t.Fatal("expected error when region is missing")
	}
}
