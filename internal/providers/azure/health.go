package azure

import (
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// mapHealth turns a token expiry into a health state and the action the
// operator should take. The window makes "expiring" a distinct, actionable
// state rather than waiting for a hard failure at expiry.
func mapHealth(tok azToken, now time.Time, window time.Duration) (domain.AccessHealth, domain.ActionHint) {
	switch {
	case !tok.ExpiresAt.After(now):
		return domain.HealthExpired, domain.ActionRenew
	case tok.ExpiresAt.Before(now.Add(window)):
		return domain.HealthExpiring, domain.ActionRenew
	default:
		return domain.HealthValid, domain.ActionUse
	}
}
