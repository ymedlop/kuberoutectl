package execx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// TraceConfig is the shared, mutable toggle for verbose command tracing. It is
// built once (before flags are parsed) and injected into a traceRunner; the CLI
// flips Enabled and points Writer at stderr in PersistentPreRunE, once the
// --verbose flag has been parsed. A pointer is shared so the already-wired
// runner observes the flag decision made later in the command lifecycle.
type TraceConfig struct {
	Enabled bool
	Writer  io.Writer
}

// traceRunner decorates a CommandRunner, emitting each invocation's command
// line, outcome, and — on failure — the command's stderr to cfg.Writer when
// tracing is enabled. It never alters the wrapped runner's results: tracing is
// observability, not behavior.
type traceRunner struct {
	inner CommandRunner
	cfg   *TraceConfig
}

// NewTraceRunner wraps inner so that, when cfg.Enabled, every command is traced
// to cfg.Writer. A nil or disabled cfg makes it a transparent passthrough.
func NewTraceRunner(inner CommandRunner, cfg *TraceConfig) CommandRunner {
	return traceRunner{inner: inner, cfg: cfg}
}

// Run delegates to the inner runner and, when tracing is on, writes a trace
// line. Results are returned unmodified regardless of tracing so verbose mode
// can never change what a command produces.
func (t traceRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	stdout, stderr, err := t.inner.Run(ctx, name, args...)
	if t.cfg == nil || !t.cfg.Enabled || t.cfg.Writer == nil {
		return stdout, stderr, err
	}
	t.trace(name, args, stderr, err)
	return stdout, stderr, err
}

// trace renders one invocation: the command line, its outcome, and the
// command's own stderr when it failed. A successful command prints no stderr
// block; a failure with empty stderr prints no stderr block either.
func (t traceRunner) trace(name string, args []string, stderr []byte, err error) {
	cmd := redactCommand(name, args)
	if err == nil {
		fmt.Fprintf(t.cfg.Writer, "[exec] %s → ok\n", cmd)
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		fmt.Fprintf(t.cfg.Writer, "[exec] %s → exit %d\n", cmd, exitErr.ExitCode())
	} else {
		fmt.Fprintf(t.cfg.Writer, "[exec] %s → error: %v\n", cmd, err)
	}
	if s := strings.TrimSpace(string(stderr)); s != "" {
		fmt.Fprintf(t.cfg.Writer, "       stderr: %s\n", s)
	}
}

// redactCommand renders a command line for tracing with the values of
// secret-bearing flags masked. Some cloud CLIs take credentials as arguments
// (notably `aws sso list-accounts --access-token <token>`), which would
// otherwise be printed verbatim under --verbose. Both `--flag value` and
// `--flag=value` forms are masked. This is the trace's only defense: stdout is
// never traced, and no known CLI echoes these tokens back on stderr.
func redactCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	maskNext := false
	for _, a := range args {
		switch {
		case maskNext:
			parts = append(parts, "***")
			maskNext = false
		case strings.HasPrefix(a, "-") && strings.Contains(a, "="):
			flag, _, _ := strings.Cut(a, "=")
			if isSecretFlag(flag) {
				parts = append(parts, flag+"=***")
			} else {
				parts = append(parts, a)
			}
		case strings.HasPrefix(a, "-") && isSecretFlag(a):
			parts = append(parts, a)
			maskNext = true // the next arg is the secret value
		default:
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

// isSecretFlag reports whether a flag name denotes credential material whose
// value must not be traced. Kept to a conservative substring set (token /
// secret / password) so benign flags are never over-masked.
func isSecretFlag(flag string) bool {
	n := strings.ToLower(strings.TrimLeft(flag, "-"))
	return strings.Contains(n, "token") ||
		strings.Contains(n, "secret") ||
		strings.Contains(n, "password") ||
		strings.Contains(n, "passwd")
}
