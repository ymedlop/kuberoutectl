// Package config resolves filesystem locations and user overrides for
// kuberoutectl. It holds no business logic — just where things live.
package config

import (
	"os"
	"path/filepath"
)

// Config holds resolved runtime configuration.
type Config struct {
	// Root is the base directory for all local state (default ~/.kuberoutectl).
	Root string

	// BinaryPaths maps a tool name (e.g. "az", "aws", "kubectl") to an
	// explicit executable path. Highest priority in binary resolution.
	BinaryPaths map[string]string
}

// Default returns a Config rooted at ~/.kuberoutectl. If the home directory
// cannot be determined it falls back to a ".kuberoutectl" directory relative
// to the current working directory.
func Default() Config {
	root := ".kuberoutectl"
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".kuberoutectl")
	}
	return Config{
		Root:        root,
		BinaryPaths: map[string]string{},
	}
}

// CacheDir is where provider-discovered inventory is persisted.
func (c Config) CacheDir() string { return filepath.Join(c.Root, "cache") }

// StateDir is where user-owned organization (labels, collections, selection)
// is persisted, separate from cache so a resync never touches it.
func (c Config) StateDir() string { return filepath.Join(c.Root, "state") }
