// Package jsonstore is the milestone-1 JSON implementation of cache.CacheStore.
//
// Layout under the configured root:
//
//	cache/
//	  snapshot.json      (provider-discovered inventory)
//	state/
//	  user-labels.json   (user-owned)
//	  collections.json   (user-owned)
//	  selection.json     (user-owned)
//
// The cache/ vs state/ split is the on-disk half of "sync must never
// overwrite user labels": SaveSnapshot only ever writes under cache/.
package jsonstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Store implements cache.CacheStore using JSON files on disk.
type Store struct {
	cacheDir string
	stateDir string
}

// New returns a Store writing inventory under cacheDir and user state under
// stateDir. Directories are created lazily on first write.
func New(cacheDir, stateDir string) *Store {
	return &Store{cacheDir: cacheDir, stateDir: stateDir}
}

const (
	snapshotFile    = "snapshot.json"
	userLabelsFile  = "user-labels.json"
	collectionsFile = "collections.json"
	selectionFile   = "selection.json"
)

// --- Snapshot (discovered inventory) ---

func (s *Store) LoadSnapshot() (domain.InventorySnapshot, error) {
	var snap domain.InventorySnapshot
	err := readJSON(filepath.Join(s.cacheDir, snapshotFile), &snap)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.InventorySnapshot{}, nil
	}
	return snap, err
}

func (s *Store) SaveSnapshot(snap domain.InventorySnapshot) error {
	return writeJSON(filepath.Join(s.cacheDir, snapshotFile), snap)
}

// --- User labels ---

func (s *Store) LoadUserLabels() (map[domain.TargetID]map[string]string, error) {
	out := map[domain.TargetID]map[string]string{}
	err := readJSON(filepath.Join(s.stateDir, userLabelsFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return map[domain.TargetID]map[string]string{}, nil
	}
	return out, err
}

func (s *Store) SaveUserLabels(labels map[domain.TargetID]map[string]string) error {
	return writeJSON(filepath.Join(s.stateDir, userLabelsFile), labels)
}

// --- Collections ---

func (s *Store) LoadCollections() ([]domain.Collection, error) {
	var out []domain.Collection
	err := readJSON(filepath.Join(s.stateDir, collectionsFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

func (s *Store) SaveCollections(cs []domain.Collection) error {
	return writeJSON(filepath.Join(s.stateDir, collectionsFile), cs)
}

// --- Selection ---

func (s *Store) LoadSelection() (domain.Selection, error) {
	var sel domain.Selection
	err := readJSON(filepath.Join(s.stateDir, selectionFile), &sel)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.Selection{}, nil
	}
	return sel, err
}

func (s *Store) SaveSelection(sel domain.Selection) error {
	return writeJSON(filepath.Join(s.stateDir, selectionFile), sel)
}

// --- helpers ---

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err // callers translate fs.ErrNotExist into an empty value
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// writeJSON writes v atomically: it writes to a temp file in the same
// directory then renames over the target, so a crash mid-write cannot leave a
// truncated cache file behind.
func writeJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if the rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp into %s: %w", path, err)
	}
	return nil
}
