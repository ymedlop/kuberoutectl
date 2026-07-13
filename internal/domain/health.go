package domain

// AccessHealth is the state of a credential or target. It is deliberately
// not a boolean: a static kubeconfig certificate is neither "valid with an
// expiry" nor "expired" — it simply does not participate in a renewal
// lifecycle. Forcing cloud semantics onto such credentials is exactly the
// modelling mistake this enum exists to avoid.
type AccessHealth string

const (
	HealthValid    AccessHealth = "valid"
	HealthExpiring AccessHealth = "expiring"
	HealthExpired  AccessHealth = "expired"
	HealthStatic   AccessHealth = "static"
	HealthUnknown  AccessHealth = "unknown"
	HealthError    AccessHealth = "error"
)

// Valid reports whether h is a recognised health state.
func (h AccessHealth) Valid() bool {
	switch h {
	case HealthValid, HealthExpiring, HealthExpired, HealthStatic, HealthUnknown, HealthError:
		return true
	default:
		return false
	}
}
