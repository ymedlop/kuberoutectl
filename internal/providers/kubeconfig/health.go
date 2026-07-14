package kubeconfig

import "github.com/ymedlop/kuberoutectl/internal/domain"

// Auth type classifications for a kubeconfig user. kuberoutectl cannot renew
// any of them, so these drive health/action but never a renewal path.
const (
	authExec       = "exec"          // externally managed (aws/gcp/oidc plugin)
	authProviderFn = "auth-provider" // legacy provider plugin, externally managed
	authClientCert = "client-cert"   // static x509 client certificate
	authToken      = "token"         // static bearer token
	authBasic      = "basic"         // static username/password
	authUnknown    = "unknown"
)

// classifyUserAuth determines how a kubeconfig user authenticates. Order
// matters: exec and auth-provider are dynamic mechanisms and take precedence
// over any static material that may also be present.
func classifyUserAuth(u kcUser) string {
	switch {
	case u.Exec != nil:
		return authExec
	case u.AuthProvider != nil:
		return authProviderFn
	case u.ClientCertificateData != "" || u.ClientCertificate != "":
		return authClientCert
	case u.Token != "" || u.TokenFile != "":
		return authToken
	case u.Username != "":
		return authBasic
	default:
		return authUnknown
	}
}

// mapKubeconfigHealth turns an auth type into health and next action. Nothing
// here is renewable by kuberoutectl:
//
//   - static material (cert / token / basic) is Health=static, Action=none —
//     exactly the case the AccessHealth doc calls out as "does not participate
//     in a renewal lifecycle".
//   - exec / auth-provider credentials are refreshed on demand by their plugin,
//     outside our view, so they are Health=unknown, Action=none rather than a
//     false "valid".
func mapKubeconfigHealth(authType string) (domain.AccessHealth, domain.ActionHint) {
	switch authType {
	case authClientCert, authToken, authBasic:
		return domain.HealthStatic, domain.ActionNone
	case authExec, authProviderFn:
		return domain.HealthUnknown, domain.ActionNone
	default:
		return domain.HealthUnknown, domain.ActionNone
	}
}
