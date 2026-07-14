package execx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// BinaryResolver locates the executable for a required third-party CLI.
type BinaryResolver interface {
	Resolve(name string) (path string, err error)
}

// LookPathFunc and StatFunc are injectable so resolution order can be tested
// without depending on the host's real PATH or filesystem.
type LookPathFunc func(file string) (string, error)
type StatFunc func(name string) (os.FileInfo, error)

// PathResolver implements the required resolution order:
//
//  1. explicit path from config,
//  2. managed runtime installed by kuberoutectl,
//  3. PATH lookup,
//  4. clear diagnostic error.
//
// Managed runtime support is optional: if no managed directory is configured,
// step 2 is simply skipped. It is never the default assumption.
type PathResolver struct {
	// ConfigPaths maps tool name -> explicit executable path (step 1).
	ConfigPaths map[string]string
	// ManagedDir is the directory kuberoutectl installs managed CLIs into
	// (step 2). Empty means no managed runtime is configured.
	ManagedDir string

	// LookPath and Stat default to the os/exec implementations; tests override.
	LookPath LookPathFunc
	Stat     StatFunc
}

// NewPathResolver builds a resolver from explicit config paths and an optional
// managed directory, wired to the real filesystem.
func NewPathResolver(configPaths map[string]string, managedDir string) *PathResolver {
	return &PathResolver{
		ConfigPaths: configPaths,
		ManagedDir:  managedDir,
		LookPath:    exec.LookPath,
		Stat:        os.Stat,
	}
}

// Resolve applies the four-step order and returns a diagnostic error naming
// each place it looked when the tool is not found.
func (r *PathResolver) Resolve(name string) (string, error) {
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	stat := r.Stat
	if stat == nil {
		stat = os.Stat
	}

	// 1. explicit config path
	if p, ok := r.ConfigPaths[name]; ok && p != "" {
		if _, err := stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("configured path for %q does not exist: %s", name, p)
	}

	// 2. managed runtime (optional)
	if r.ManagedDir != "" {
		candidate := filepath.Join(r.ManagedDir, name)
		if _, err := stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 3. PATH lookup
	if p, err := lookPath(name); err == nil {
		return p, nil
	}

	// 4. clear diagnostic error
	return "", fmt.Errorf(
		"could not resolve binary %q: not in config paths, not in managed runtime (%s), and not found on PATH",
		name, managedDirLabel(r.ManagedDir),
	)
}

func managedDirLabel(dir string) string {
	if dir == "" {
		return "not configured"
	}
	return dir
}
