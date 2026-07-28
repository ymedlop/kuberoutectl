package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Evaluate is the composition of the fetch and the comparison, and it is the
// only piece both commands call. The layers below it are tested separately —
// Latest against every failure mode, Newer against every version shape — so what
// is tested here is specifically that composing them cannot produce a verdict
// neither half supports.
//
// The case that matters most is the last one: a failure must yield
// VerdictUnknown, never VerdictCurrent. "We could not reach GitHub" and "you are
// on the newest release" are different facts, and only one of them is a claim
// about the user's build.
func TestEvaluate(t *testing.T) {
	cases := []struct {
		name        string
		current     string
		handler     http.HandlerFunc
		wantVerdict Verdict
		wantLatest  string
	}{
		{
			name:    "a newer release",
			current: "1.0.0",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","prerelease":false}`))
			},
			wantVerdict: VerdictOutdated,
			wantLatest:  "v1.2.0",
		},
		{
			name:    "already on the newest",
			current: "1.2.0",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","prerelease":false}`))
			},
			wantVerdict: VerdictCurrent,
			wantLatest:  "v1.2.0",
		},
		{
			name:    "ahead of the newest published release",
			current: "1.3.0",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","prerelease":false}`))
			},
			wantVerdict: VerdictCurrent,
			wantLatest:  "v1.2.0",
		},
		{
			name:    "a build with no comparable version",
			current: "0.0.0-snapshot-abc1234",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","prerelease":false}`))
			},
			wantVerdict: VerdictUnknown,
		},
		{
			name:        "rate limited",
			current:     "1.0.0",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
			wantVerdict: VerdictUnknown,
		},
		{
			name:    "malformed body must not read as up to date",
			current: "1.0.0",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("{not json"))
			},
			wantVerdict: VerdictUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			got := New(srv.Client(), srv.URL).Evaluate(context.Background(), tc.current)
			if got.Verdict != tc.wantVerdict {
				t.Errorf("verdict = %v, want %v (reason %q)", got.Verdict, tc.wantVerdict, got.Reason)
			}
			if got.Current != tc.current {
				t.Errorf("Current = %q, want %q", got.Current, tc.current)
			}
			if got.Latest != tc.wantLatest {
				t.Errorf("Latest = %q, want %q", got.Latest, tc.wantLatest)
			}
			// Reason and Latest are mutually exclusive: a verdict either rests on a
			// comparison or explains why there was none.
			if (got.Verdict == VerdictUnknown) != (got.Reason != "") {
				t.Errorf("verdict %v with reason %q — a reason must accompany exactly the unknown verdict", got.Verdict, got.Reason)
			}
		})
	}
}
