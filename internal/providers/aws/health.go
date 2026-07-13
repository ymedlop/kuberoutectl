package aws

import (
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// Auth type classifications. AWS renewal semantics depend entirely on which of
// these a profile uses, so classification drives both health and the action
// hint.
const (
	authSSO     = "sso"
	authRole    = "role"
	authStatic  = "static"
	authUnknown = "unknown"
)

// classifyAuth determines a profile's auth type. SSO configuration wins because
// an SSO-backed role still presents an assumed-role ARN; the presence of an SSO
// start URL is the reliable signal that it is renewable via `aws sso login`.
func classifyAuth(ssoStartURL, arn string, stsOK bool) string {
	if ssoStartURL != "" {
		return authSSO
	}
	if stsOK {
		switch {
		case strings.Contains(arn, ":user/"):
			return authStatic
		case strings.Contains(arn, ":assumed-role/"):
			return authRole
		}
	}
	return authUnknown
}

// mapAWSHealth turns auth type + STS result into health and next action.
//
// The important distinction from Azure: a working static-key profile is not
// "valid pending expiry" — it is `static` with nothing to do, and a failing
// static profile is an `error` needing a `manual` fix (edit credentials), not a
// renewal. Only SSO/role profiles map failure to renew.
func mapAWSHealth(authType string, stsOK bool) (domain.AccessHealth, domain.ActionHint) {
	if stsOK {
		switch authType {
		case authStatic:
			return domain.HealthStatic, domain.ActionNone
		default: // sso, role, unknown-but-working
			return domain.HealthValid, domain.ActionUse
		}
	}
	switch authType {
	case authSSO, authRole:
		return domain.HealthExpired, domain.ActionRenew
	case authStatic:
		return domain.HealthError, domain.ActionManual
	default:
		return domain.HealthUnknown, domain.ActionManual
	}
}
