package domain

// AccessCheckMode records what an operability check was able to establish about
// a target, which is a different question from whether any given credential
// passed it.
//
// EKS clusters authorize in two independent places: IAM (can you call the API
// at all) and the cluster itself (are you admitted to Kubernetes). A cluster's
// authentication mode decides whether the second one is *readable*, and that is
// what this records — the mode, not the answer.
type AccessCheckMode string

const (
	// AccessCheckAPI means access entries are the only authorization path, so
	// the entry list is authoritative in both directions: a credential absent
	// from it will be refused. This is the only mode that permits a negative.
	AccessCheckAPI AccessCheckMode = "api"
	// AccessCheckAPIAndConfigMap means entries coexist with the legacy aws-auth
	// ConfigMap. Presence still proves admission; absence proves nothing,
	// because the ConfigMap may grant it and kuberoutectl does not read it.
	AccessCheckAPIAndConfigMap AccessCheckMode = "api_and_config_map"
	// AccessCheckConfigMap means access entries do not apply to this cluster at
	// all. Nothing can be concluded either way. Clusters created through the
	// API, the SDKs or CloudFormation default to this, so it is the common case,
	// not an error case.
	AccessCheckConfigMap AccessCheckMode = "config_map"
	// AccessCheckUnavailable means the check was attempted and failed — most
	// often a missing eks:ListAccessEntries permission. Recorded distinctly from
	// "not attempted" so an operator can tell a gap in permissions from a
	// cluster nobody needed to check.
	AccessCheckUnavailable AccessCheckMode = "unavailable"
)

// Valid reports whether m is a mode this build understands. An unrecognised
// mode is deliberately not treated as AccessCheckAPI: `api` is the only mode
// allowed to produce a negative verdict, so guessing wrong there turns silence
// into a confident refusal.
func (m AccessCheckMode) Valid() bool {
	switch m {
	case AccessCheckAPI, AccessCheckAPIAndConfigMap, AccessCheckConfigMap, AccessCheckUnavailable:
		return true
	default:
		return false
	}
}

// AccessVerdict is what can be said about one credential's ability to operate
// inside one target. Three values, not two: "we could not tell" is a real
// answer here and collapsing it into "no" is the failure this whole feature
// exists to avoid.
type AccessVerdict string

const (
	AccessOperable    AccessVerdict = "operable"
	AccessNotOperable AccessVerdict = "not operable"
	AccessUnknown     AccessVerdict = "unknown"
)

// AccessVerdictFor derives a verdict from membership in the operable set plus
// the mode under which that set was read. Both inputs are required: membership
// alone cannot distinguish "confirmed refused" from "we could not see".
//
// Presence is trustworthy under every mode. Absence is only meaningful under
// AccessCheckAPI.
func AccessVerdictFor(listed bool, check AccessCheckMode) AccessVerdict {
	if listed {
		return AccessOperable
	}
	if check == AccessCheckAPI {
		return AccessNotOperable
	}
	return AccessUnknown
}

// CredentialAccess reports what is known about reaching *into* this target with
// the given credential, as opposed to Health, which reports whether the
// credential can authenticate to the provider at all. A credential can be
// perfectly valid and still be refused by the cluster.
//
// The verdict is derived on read from OperableCredentialIDs and AccessCheck
// rather than stored per (target, credential) pair, so the two can never drift
// apart into a stored contradiction.
func (t Target) CredentialAccess(id CredentialID) AccessVerdict {
	listed := false
	for _, c := range t.OperableCredentialIDs {
		if c == id {
			listed = true
			break
		}
	}
	return AccessVerdictFor(listed, t.AccessCheck)
}
