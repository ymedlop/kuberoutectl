// Package cache abstracts local persistence. The CacheStore interface keeps
// services independent of the storage format; the milestone-1 implementation
// is JSON (see jsonstore). SQLite is deliberately not assumed.
package cache

import "github.com/ymedlop/kuberoutectl/internal/domain"

// CacheStore persists kuberoutectl's local state. Discovered inventory and
// user-owned organization are loaded/saved through separate methods because
// they live in separate files — a resync replaces the snapshot without ever
// touching user labels, collections, or selection.
type CacheStore interface {
	LoadSnapshot() (domain.InventorySnapshot, error)
	SaveSnapshot(domain.InventorySnapshot) error

	LoadUserLabels() (map[domain.TargetID]map[string]string, error)
	SaveUserLabels(map[domain.TargetID]map[string]string) error

	LoadCollections() ([]domain.Collection, error)
	SaveCollections([]domain.Collection) error

	LoadSelection() (domain.Selection, error)
	SaveSelection(domain.Selection) error
}
