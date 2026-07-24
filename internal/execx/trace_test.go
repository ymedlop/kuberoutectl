package execx

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestTraceRunner_DisabledPassthrough: a disabled tracer writes nothing and
// returns the wrapped runner's stdout/stderr/err unchanged.
func TestTraceRunner_DisabledPassthrough(t *testing.T) {
	fake := NewFakeRunner()
	fake.Responses["echo hi"] = FakeResponse{Stdout: []byte("out"), Stderr: []byte("err")}
	var buf bytes.Buffer
	r := NewTraceRunner(fake, &TraceConfig{Enabled: false, Writer: &buf})

	stdout, stderr, err := r.Run(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(stdout) != "out" || string(stderr) != "err" {
		t.Errorf("passthrough mismatch: stdout=%q stderr=%q", stdout, stderr)
	}
	if buf.Len() != 0 {
		t.Errorf("disabled tracer wrote output: %q", buf.String())
	}
}

// TestTraceRunner_NilConfig: a nil config is a transparent passthrough and must
// not panic.
func TestTraceRunner_NilConfig(t *testing.T) {
	fake := NewFakeRunner()
	fake.Responses["echo hi"] = FakeResponse{Stdout: []byte("out")}
	r := NewTraceRunner(fake, nil)
	stdout, _, err := r.Run(context.Background(), "echo", "hi")
	if err != nil || string(stdout) != "out" {
		t.Fatalf("nil-config passthrough failed: stdout=%q err=%v", stdout, err)
	}
}

// TestTraceRunner_SuccessTrace: an enabled tracer records the command line and
// an "ok" outcome, with no stderr block.
func TestTraceRunner_SuccessTrace(t *testing.T) {
	var buf bytes.Buffer
	r := NewTraceRunner(NewExecRunner(), &TraceConfig{Enabled: true, Writer: &buf})

	if _, _, err := r.Run(context.Background(), "sh", "-c", "exit 0"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[exec] sh -c exit 0") || !strings.Contains(got, "→ ok") {
		t.Errorf("success trace missing command/ok: %q", got)
	}
	if strings.Contains(got, "stderr:") {
		t.Errorf("success trace should have no stderr block: %q", got)
	}
}

// TestTraceRunner_FailureWithStderr: a real non-zero exit is traced with its
// exit code and the command's stderr — the expired-token case in miniature.
func TestTraceRunner_FailureWithStderr(t *testing.T) {
	var buf bytes.Buffer
	r := NewTraceRunner(NewExecRunner(), &TraceConfig{Enabled: true, Writer: &buf})

	if _, _, err := r.Run(context.Background(), "sh", "-c", "echo boom >&2; exit 7"); err == nil {
		t.Fatal("expected non-zero exit error")
	}
	got := buf.String()
	if !strings.Contains(got, "→ exit 7") {
		t.Errorf("expected exit code 7 in trace: %q", got)
	}
	if !strings.Contains(got, "stderr: boom") {
		t.Errorf("expected stderr in trace: %q", got)
	}
}

// TestTraceRunner_RedactsSecretArgs: secret-bearing flag values (e.g. the AWS
// SSO --access-token) must never appear in the trace, in either flag form.
func TestTraceRunner_RedactsSecretArgs(t *testing.T) {
	fake := NewFakeRunner()
	const secret = "eyJ-super-secret-sso-token"
	key := "aws sso list-accounts --access-token " + secret + " --region eu-west-1 --output json"
	fake.Responses[key] = FakeResponse{Stdout: []byte("{}")}
	var buf bytes.Buffer
	r := NewTraceRunner(fake, &TraceConfig{Enabled: true, Writer: &buf})

	if _, _, err := r.Run(context.Background(), "aws", "sso", "list-accounts",
		"--access-token", secret, "--region", "eu-west-1", "--output", "json"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, secret) {
		t.Errorf("trace leaked the access token: %q", got)
	}
	if !strings.Contains(got, "--access-token ***") {
		t.Errorf("expected masked token in trace: %q", got)
	}
	// Non-secret args around it must survive intact.
	if !strings.Contains(got, "--region eu-west-1") {
		t.Errorf("non-secret args should be preserved: %q", got)
	}
}

// TestRedactCommand covers the pure redaction helper directly, including the
// inline --flag=value form and that benign flags are left untouched.
func TestRedactCommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"az", []string{"account", "list", "--output", "json"}, "az account list --output json"},
		{"aws", []string{"sso", "list-accounts", "--access-token", "SEKRET"}, "aws sso list-accounts --access-token ***"},
		{"aws", []string{"login", "--password=SEKRET"}, "aws login --password=***"},
		{"aws", []string{"x", "--client-secret", "SEKRET", "--region", "eu"}, "aws x --client-secret *** --region eu"},
		{"aws", []string{"configure", "get", "region", "--profile", "default"}, "aws configure get region --profile default"},
	}
	for _, c := range cases {
		if got := redactCommand(c.name, c.args); got != c.want {
			t.Errorf("redactCommand(%q, %v) = %q, want %q", c.name, c.args, got, c.want)
		}
	}
}

// TestTraceRunner_FailureEmptyStderr: a failure with empty stderr emits the
// exit line but no stderr block (edge case 3).
func TestTraceRunner_FailureEmptyStderr(t *testing.T) {
	var buf bytes.Buffer
	r := NewTraceRunner(NewExecRunner(), &TraceConfig{Enabled: true, Writer: &buf})

	if _, _, err := r.Run(context.Background(), "sh", "-c", "exit 5"); err == nil {
		t.Fatal("expected non-zero exit error")
	}
	got := buf.String()
	if !strings.Contains(got, "→ exit 5") {
		t.Errorf("expected exit code 5 in trace: %q", got)
	}
	if strings.Contains(got, "stderr:") {
		t.Errorf("empty stderr must produce no stderr block: %q", got)
	}
}
