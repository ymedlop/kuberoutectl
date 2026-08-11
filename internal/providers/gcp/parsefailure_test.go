package gcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// progressRecorder captures Progress.Step calls.
type progressRecorder struct{ steps []string }

func (p *progressRecorder) Step(format string, args ...any) {
	p.steps = append(p.steps, fmt.Sprintf(format, args...))
}

// discoverClusters deliberately skips a project whose cluster listing will not
// parse, so one malformed project cannot sink the whole sync. That resilience is
// right; the silence is not. The command succeeded, so `--verbose` shows nothing
// either, and a gcloud output-format change reads as "this project has no
// clusters".
func TestDiscover_MalformedClusterListIsReported(t *testing.T) {
	r := fullRunner(t)
	r.Responses["gcloud container clusters list --project platform-lab-456 --format=json"] = execx.FakeResponse{Stdout: []byte("{{ not json")}
	prog := &progressRecorder{}

	res, err := New(fakeResolver{path: "gcloud"}, r).Discover(context.Background(), providers.DiscoveryInput{Progress: prog})
	if err != nil {
		t.Fatalf("Discover must stay resilient, got: %v", err)
	}
	// The healthy project's clusters must still be discovered.
	if len(res.Targets) == 0 {
		t.Fatal("the healthy project's clusters must survive a sibling's parse failure")
	}
	for _, tg := range res.Targets {
		if strings.Contains(string(tg.ID), "platform-lab-456") {
			t.Errorf("unparseable project yielded a target: %q", tg.ID)
		}
	}

	var found bool
	for _, s := range prog.steps {
		if strings.Contains(strings.ToLower(s), "parse") && strings.Contains(s, "platform-lab-456") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no progress step names the project whose listing could not be parsed; steps:\n%s",
			strings.Join(prog.steps, "\n"))
	}
}
