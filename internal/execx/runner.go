// Package execx isolates all external-process execution and binary path
// resolution behind interfaces. Provider code depends on these interfaces so
// discovery/renewal logic can be unit-tested against captured fixtures with a
// fake runner — no real cloud CLI required in CI.
package execx

import (
	"bytes"
	"context"
	"os/exec"
)

// CommandRunner executes an external command and returns its stdout and
// stderr separately. Keeping this an interface is what makes provider parsing
// testable without shelling out.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// ExecRunner is the real CommandRunner backed by os/exec.
type ExecRunner struct{}

// NewExecRunner returns a CommandRunner that runs actual processes.
func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// Run executes name with args, capturing stdout and stderr independently.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return out.Bytes(), errBuf.Bytes(), err
}
