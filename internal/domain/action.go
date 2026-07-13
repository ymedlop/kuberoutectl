package domain

// ActionHint is the single next action kuberoutectl suggests to the
// operator for a credential or target. It keeps the CLI more useful than a
// raw inventory dump: instead of "expired", the user sees "renew".
type ActionHint string

const (
	ActionUse    ActionHint = "use"
	ActionRenew  ActionHint = "renew"
	ActionSwitch ActionHint = "switch"
	ActionRepair ActionHint = "repair"
	ActionManual ActionHint = "manual"
	ActionNone   ActionHint = "none"
)

// Valid reports whether a is a recognised action hint.
func (a ActionHint) Valid() bool {
	switch a {
	case ActionUse, ActionRenew, ActionSwitch, ActionRepair, ActionManual, ActionNone:
		return true
	default:
		return false
	}
}
