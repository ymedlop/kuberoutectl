package domain

// Collection is a saved view over targets, not a folder. Membership is
// primarily selector-driven (so newly discovered targets that match join
// automatically), with optional explicit StaticIDs for the cases a selector
// cannot express.
type Collection struct {
	ID          CollectionID  `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Selector    LabelSelector `json:"selector"`
	StaticIDs   []TargetID    `json:"static_ids,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
}
