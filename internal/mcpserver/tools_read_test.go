package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

func TestListTargets_FilterByProviderAndSelector(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	ctx := context.Background()

	_, all, err := h.listTargets(ctx, nil, ListTargetsInput{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(all.Targets))
	}

	_, aws, err := h.listTargets(ctx, nil, ListTargetsInput{Provider: "aws"})
	if err != nil {
		t.Fatalf("list aws: %v", err)
	}
	if len(aws.Targets) != 1 || aws.Targets[0].ProviderID != "aws" {
		t.Fatalf("provider filter failed: %+v", aws.Targets)
	}

	_, prod, err := h.listTargets(ctx, nil, ListTargetsInput{Selector: "env=prod"})
	if err != nil {
		t.Fatalf("list selector: %v", err)
	}
	if len(prod.Targets) != 1 || prod.Targets[0].ID != "aws:eks:prod" {
		t.Fatalf("selector filter failed: %+v", prod.Targets)
	}
}

func TestGetTarget_ByRefAndMissing(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	ctx := context.Background()

	_, got, err := h.getTarget(ctx, nil, GetTargetInput{Ref: "eks-prod"})
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.Target.ID != "aws:eks:prod" {
		t.Errorf("resolved wrong target: %s", got.Target.ID)
	}

	if _, _, err := h.getTarget(ctx, nil, GetTargetInput{Ref: "does-not-exist"}); err == nil {
		t.Error("expected error for unknown ref")
	}
}

func TestReadTools_EmptyCache(t *testing.T) {
	h := newTestHandler(t, domain.InventorySnapshot{})
	ctx := context.Background()

	if _, out, err := h.listTargets(ctx, nil, ListTargetsInput{}); err != nil || len(out.Targets) != 0 {
		t.Errorf("empty list_targets = (%+v, %v), want empty/no-error", out.Targets, err)
	}
	if _, out, err := h.listCredentials(ctx, nil, ListCredentialsInput{}); err != nil || len(out.Credentials) != 0 {
		t.Errorf("empty list_credentials = (%+v, %v), want empty/no-error", out.Credentials, err)
	}
	if _, _, err := h.getStatus(ctx, nil, GetStatusInput{}); err != nil {
		t.Errorf("get_status on empty cache errored: %v", err)
	}
}

// TestListCredentials_NoSecrets guards the "no secrets" requirement: the tool's
// JSON output must not carry any secret-looking field.
func TestListCredentials_NoSecrets(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	_, out, err := h.listCredentials(context.Background(), nil, ListCredentialsInput{})
	if err != nil {
		t.Fatalf("list_credentials: %v", err)
	}
	if len(out.Credentials) == 0 {
		t.Fatal("expected at least one credential to check")
	}
	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	low := strings.ToLower(string(blob))
	for _, bad := range []string{"token", "secret", "password", "access_key", "private_key", "client_secret"} {
		if strings.Contains(low, bad) {
			t.Errorf("credential output leaked a secret-like field %q: %s", bad, blob)
		}
	}
}

func TestListProviders(t *testing.T) {
	h := newTestHandler(t, seedTargets(), &fakeProvider{id: "aws", caps: domain.Capabilities{CanRenew: true}})
	_, out, err := h.listProviders(context.Background(), nil, ListProvidersInput{})
	if err != nil {
		t.Fatalf("list_providers: %v", err)
	}
	if len(out.Providers) != 1 || out.Providers[0].ID != "aws" || !out.Providers[0].Capabilities.CanRenew {
		t.Errorf("unexpected providers: %+v", out.Providers)
	}
}
