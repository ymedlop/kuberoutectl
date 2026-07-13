package domain

import "time"

// InventorySnapshot is the persisted local cache of discovered state. It
// holds ONLY provider-discovered data — user labels, collections, and
// selection live in separate state files so that a resync (which replaces the
// snapshot) can never clobber user-owned organization.
type InventorySnapshot struct {
	Sources     []AccessSource `json:"sources"`
	Credentials []Credential   `json:"credentials"`
	Scopes      []Scope        `json:"scopes"`
	Targets     []Target       `json:"targets"`
	SyncedAt    time.Time      `json:"synced_at"`
}

// Selection records the operator's current target/collection choice.
type Selection struct {
	TargetID     TargetID     `json:"target_id,omitempty"`
	CollectionID CollectionID `json:"collection_id,omitempty"`
	UpdatedAt    time.Time    `json:"updated_at"`
}
