package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// The happy path: a real-shaped payload yields the tag, and the request carries
// the Accept header and nothing else — no version, no identifier, no query
// string. The tool must be able to claim it transmits nothing, and that claim is
// worth an assertion rather than a sentence in the docs.
func TestLatest_ReadsTheTagAndSendsNothing(t *testing.T) {
	var gotAccept, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept, gotQuery, gotAuth = r.Header.Get("Accept"), r.URL.RawQuery, r.Header.Get("Authorization")
		_, _ = w.Write(fixture(t, "release-latest.json"))
	}))
	defer srv.Close()

	version, ok, reason := New(srv.Client(), srv.URL).Latest(context.Background())
	if !ok {
		t.Fatalf("Latest failed on a good payload: %s", reason)
	}
	if version != "v1.2.0" {
		t.Errorf("version = %q, want %q", version, "v1.2.0")
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if gotQuery != "" {
		t.Errorf("request carried a query string %q; it must transmit nothing", gotQuery)
	}
	if gotAuth != "" {
		t.Errorf("request carried an Authorization header; it must be unauthenticated")
	}
}

// Every failure mode. The verdict is always "could not tell", never "up to
// date": reporting a network problem as a clean bill of health is the
// update-check form of the parse-failure bug fixed in #111, and it is
// indistinguishable to the user from the truth.
//
// Each case also asserts a non-empty reason, because the row that gets rendered
// is the reason — an empty one produces a check that says nothing at all.
func TestLatest_FailureModes(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "rate limited — the likely failure behind a corporate NAT",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
		},
		{
			name:    "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		},
		{
			name:    "not found — a renamed or deleted repo",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
		},
		{
			name: "malformed body on a 200",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("{not json"))
			},
		},
		{
			name: "prerelease payload must never be offered",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"v1.3.0-rc.1","prerelease":true}`))
			},
		},
		{
			name: "empty tag",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tag_name":"","prerelease":false}`))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			version, ok, reason := New(srv.Client(), srv.URL).Latest(context.Background())
			if ok {
				t.Fatalf("Latest reported success (%q) on a failure case", version)
			}
			if version != "" {
				t.Errorf("version = %q, want empty on failure", version)
			}
			if reason == "" {
				t.Error("no reason given; the rendered row would say nothing")
			}
		})
	}
}

// Offline and air-gapped: no panic, no hang, a reason to show. The server is
// closed before the call so the connection is refused immediately.
func TestLatest_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	if _, ok, reason := New(&http.Client{Timeout: time.Second}, url).Latest(context.Background()); ok || reason == "" {
		t.Errorf("unreachable endpoint: ok=%v reason=%q", ok, reason)
	}
}

// A cancelled context must return, not block. This is what keeps `doctor` from
// hanging past its budget on a network that accepts the connection and then
// says nothing.
func TestLatest_RespectsContextCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	defer func() { close(block); srv.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, ok, reason := New(srv.Client(), srv.URL).Latest(ctx); ok || reason == "" {
			t.Errorf("cancelled request: ok=%v reason=%q", ok, reason)
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Latest did not return after its context was cancelled")
	}
}
