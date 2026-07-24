package mcpserver

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

func TestUseTarget_RecordsSelectionAndActivates(t *testing.T) {
	prov := &fakeProvider{id: "aws", caps: domain.Capabilities{CanSwitchContext: true}}
	h := newTestHandler(t, seedTargets(), prov)
	ctx := context.Background()

	_, out, err := h.useTarget(ctx, nil, UseTargetInput{Ref: "eks-prod", Activate: true})
	if err != nil {
		t.Fatalf("use_target: %v", err)
	}
	if out.Target.ID != "aws:eks:prod" || !out.Activated {
		t.Errorf("unexpected use_target output: %+v", out)
	}
	if prov.activated == nil || prov.activated.ID != "aws:eks:prod" {
		t.Errorf("provider Activate was not called for the target")
	}

	// get_status must now reflect the selection.
	_, st, err := h.getStatus(ctx, nil, GetStatusInput{})
	if err != nil {
		t.Fatalf("get_status: %v", err)
	}
	if st.Status.Target == nil || st.Status.Target.ID != "aws:eks:prod" {
		t.Errorf("status does not reflect the used target: %+v", st.Status)
	}
}

func TestSyncProvider_PopulatesCache(t *testing.T) {
	prov := &fakeProvider{
		id: "gcp",
		result: providers.DiscoveryResult{
			Sources:     []domain.AccessSource{{ID: "gcp:src", ProviderID: "gcp"}},
			Credentials: []domain.Credential{{ID: "gcp:cred", ProviderID: "gcp"}},
			Scopes:      []domain.Scope{{ID: "gcp:proj", ProviderID: "gcp"}},
			Targets:     []domain.Target{{ID: "gcp:gke:1", Name: "gke-1", ProviderID: "gcp"}},
		},
	}
	h := newTestHandler(t, domain.InventorySnapshot{}, prov)
	ctx := context.Background()

	_, out, err := h.syncProvider(ctx, nil, SyncProviderInput{Provider: "gcp"})
	if err != nil {
		t.Fatalf("sync_provider: %v", err)
	}
	if out.Sources != 1 || out.Credentials != 1 || out.Scopes != 1 || out.Targets != 1 {
		t.Errorf("unexpected sync counts: %+v", out)
	}

	// The cache is now populated: list_targets sees the discovered target.
	_, lt, err := h.listTargets(ctx, nil, ListTargetsInput{Provider: "gcp"})
	if err != nil {
		t.Fatalf("list_targets: %v", err)
	}
	if len(lt.Targets) != 1 || lt.Targets[0].ID != "gcp:gke:1" {
		t.Errorf("sync did not populate the cache: %+v", lt.Targets)
	}
}

func TestCreateOrUpdateCollection_Upsert(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	ctx := context.Background()

	if _, _, err := h.createOrUpdateCollection(ctx, nil, CreateOrUpdateCollectionInput{Name: "envs", Description: "first", Selector: "env=prod"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, out, err := h.createOrUpdateCollection(ctx, nil, CreateOrUpdateCollectionInput{Name: "envs", Description: "second", Selector: "env=lab"}); err != nil {
		t.Fatalf("update: %v", err)
	} else if out.Collection.Description != "second" {
		t.Errorf("upsert did not update: %+v", out.Collection)
	}

	_, list, err := h.listCollections(ctx, nil, ListCollectionsInput{})
	if err != nil {
		t.Fatalf("list_collections: %v", err)
	}
	if len(list.Collections) != 1 {
		t.Errorf("expected 1 collection after upsert, got %d", len(list.Collections))
	}
}

// TestWriteTools_Serialized runs many concurrent mutating calls; the handler
// mutex must serialize the load→mutate→save sequences so no update is lost.
// Without the lock, concurrent create_or_update_collection calls would race and
// the final count would drop below N. Run with -race for the full guarantee.
func TestWriteTools_Serialized(t *testing.T) {
	h := newTestHandler(t, seedTargets())
	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := h.createOrUpdateCollection(context.Background(), nil,
				CreateOrUpdateCollectionInput{Name: fmt.Sprintf("c%02d", i), Selector: "env=prod"})
			if err != nil {
				t.Errorf("concurrent upsert %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	_, list, err := h.listCollections(context.Background(), nil, ListCollectionsInput{})
	if err != nil {
		t.Fatalf("list_collections: %v", err)
	}
	if len(list.Collections) != n {
		t.Errorf("lost updates under concurrency: got %d collections, want %d", len(list.Collections), n)
	}
}
