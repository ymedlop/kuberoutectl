package execx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFS builds Stat/LookPath funcs over a set of "existing" paths and a PATH
// lookup table, so resolution order can be tested deterministically.
type fakeFS struct {
	exists   map[string]bool
	pathTool map[string]string // tool name -> resolved PATH location
}

func (f fakeFS) stat(name string) (os.FileInfo, error) {
	if f.exists[name] {
		return fakeInfo{}, nil
	}
	return nil, os.ErrNotExist
}

func (f fakeFS) lookPath(file string) (string, error) {
	if p, ok := f.pathTool[file]; ok {
		return p, nil
	}
	return "", errors.New("not found on PATH")
}

type fakeInfo struct{ os.FileInfo }

func newResolver(fs fakeFS, configPaths map[string]string, managedDir string) *PathResolver {
	return &PathResolver{
		ConfigPaths: configPaths,
		ManagedDir:  managedDir,
		LookPath:    fs.lookPath,
		Stat:        fs.stat,
	}
}

// Config path wins over managed runtime and PATH.
func TestResolve_ConfigPathHighestPriority(t *testing.T) {
	managed := filepath.Join("managed", "az")
	fs := fakeFS{
		exists:   map[string]bool{"/opt/az": true, managed: true},
		pathTool: map[string]string{"az": "/usr/bin/az"},
	}
	r := newResolver(fs, map[string]string{"az": "/opt/az"}, "managed")
	got, err := r.Resolve("az")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/opt/az" {
		t.Errorf("got %q, want config path /opt/az", got)
	}
}

// Configured-but-missing path is a hard error, not a silent fallthrough.
func TestResolve_ConfigPathMissingIsError(t *testing.T) {
	fs := fakeFS{
		exists:   map[string]bool{},
		pathTool: map[string]string{"az": "/usr/bin/az"},
	}
	r := newResolver(fs, map[string]string{"az": "/opt/az"}, "")
	if _, err := r.Resolve("az"); err == nil {
		t.Fatal("expected error for configured-but-missing path")
	}
}

// Managed runtime wins over PATH when no config path is set.
func TestResolve_ManagedBeatsPath(t *testing.T) {
	managed := filepath.Join("managed", "az")
	fs := fakeFS{
		exists:   map[string]bool{managed: true},
		pathTool: map[string]string{"az": "/usr/bin/az"},
	}
	r := newResolver(fs, nil, "managed")
	got, err := r.Resolve("az")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != managed {
		t.Errorf("got %q, want managed %q", got, managed)
	}
}

// PATH is the fallback when config and managed are absent.
func TestResolve_PathFallback(t *testing.T) {
	fs := fakeFS{
		exists:   map[string]bool{},
		pathTool: map[string]string{"aws": "/usr/local/bin/aws"},
	}
	r := newResolver(fs, nil, "")
	got, err := r.Resolve("aws")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/usr/local/bin/aws" {
		t.Errorf("got %q, want PATH result", got)
	}
}

// Nothing anywhere -> diagnostic error naming the places searched.
func TestResolve_NotFoundIsDiagnostic(t *testing.T) {
	fs := fakeFS{exists: map[string]bool{}, pathTool: map[string]string{}}
	r := newResolver(fs, nil, "")
	_, err := r.Resolve("kubectl")
	if err == nil {
		t.Fatal("expected error when binary is nowhere")
	}
	if !strings.Contains(err.Error(), "kubectl") || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("error not diagnostic enough: %v", err)
	}
}
