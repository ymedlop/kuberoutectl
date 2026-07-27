package azure

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

// A subscription whose `az aks list` succeeds but returns unparseable output is
// skipped rather than failing the sync — one malformed subscription must not
// sink the whole inventory. But the skip must not be silent: the command
// succeeded, so --verbose shows nothing wrong either, and an az output-format
// change would read as "this subscription has no clusters".
func TestDiscover_MalformedClusterListIsReported(t *testing.T) {
	p, r := newFakeAzProviderWithRunner(t)
	// Break exactly one subscription's listing; the fixture has more than one.
	broken := ""
	for key := range r.Responses {
		if strings.HasPrefix(key, "az aks list --subscription ") {
			broken = key
			break
		}
	}
	if broken == "" {
		t.Fatal("fixture has no `az aks list` response to break; this test would prove nothing")
	}
	r.Responses[broken] = execx.FakeResponse{Stdout: []byte("[{ not json")}

	prog := &progressRecorder{}
	if _, err := p.Discover(context.Background(), providers.DiscoveryInput{Progress: prog}); err != nil {
		t.Fatalf("Discover must stay resilient, got: %v", err)
	}

	for _, s := range prog.steps {
		if strings.Contains(strings.ToLower(s), "parse") {
			return
		}
	}
	t.Errorf("no progress step reports the unparseable cluster listing; steps:\n%s",
		strings.Join(prog.steps, "\n"))
}
