package execx

import (
	"context"
	"fmt"
)

// FakeRunner is a CommandRunner for tests. It returns canned output keyed by
// command, so provider discovery/parse logic can be exercised without any
// real CLI.
type FakeRunner struct {
	// Responses maps a command key ("name arg1 arg2") to its result.
	Responses map[string]FakeResponse
	// Calls records every invocation in order, for assertions.
	Calls []string
}

// FakeResponse is a canned command result.
type FakeResponse struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// NewFakeRunner returns an empty FakeRunner.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{Responses: map[string]FakeResponse{}}
}

// Run records the call and returns the matching canned response, or an error
// if none is registered.
func (f *FakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	f.Calls = append(f.Calls, key)
	resp, ok := f.Responses[key]
	if !ok {
		return nil, nil, fmt.Errorf("fake runner: no response registered for %q", key)
	}
	return resp.Stdout, resp.Stderr, resp.Err
}
