package aws

import (
	"encoding/json"
	"fmt"
	"strings"
)

// awsAccessEntries is the subset of `aws eks list-access-entries` we consume:
// a flat array of IAM principal ARNs plus a pagination token.
type awsAccessEntries struct {
	AccessEntries []string `json:"accessEntries"`
	NextToken     string   `json:"nextToken"`
}

// parseAccessEntries decodes one page of `list-access-entries`.
//
// A malformed body from a *successful* command is a hard error, per the
// provider error convention. It matters more here than almost anywhere else:
// degrading it to an empty list would report "nobody has an access entry",
// which under `API` mode is a confident claim that every profile will be
// refused — a format regression dressed up as an authorization fact.
func parseAccessEntries(data []byte) (awsAccessEntries, error) {
	var out awsAccessEntries
	if err := json.Unmarshal(data, &out); err != nil {
		return awsAccessEntries{}, fmt.Errorf("decode eks list-access-entries: %w", err)
	}
	return out, nil
}

// principalKey reduces an ARN to `account/name`, the form in which an access
// entry and an STS caller identity can be compared at all.
//
// They are never equal as strings: an entry names an IAM principal, STS returns
// the assumed-role it produced.
//
//	entry (SSO)    arn:aws:iam::123:role/aws-reserved/sso.amazonaws.com/eu-central-1/AWSReservedSSO_X
//	entry (plain)  arn:aws:iam::123:role/PlatformAdmin
//	caller         arn:aws:sts::123:assumed-role/AWSReservedSSO_X/yeray
//
// The reduction is asymmetric, and that is the whole trap. An assumed-role ARN
// is `assumed-role/NAME/session`, so the name is the FIRST segment; a role or
// user ARN is `role/…path…/NAME`, so the name is the LAST. Taking the last
// segment in both cases works for plain roles and IAM users and breaks for SSO
// — the only case this feature exists for, because the IAM path is present
// exactly there.
//
// Anything it cannot reduce yields "", which callers must treat as "no key" and
// therefore "unknown". Failing closed here means returning nothing rather than
// guessing a key: a wrong key matches no entry, and under `API` mode that reads
// as a confirmed refusal.
//
// Known limitation (spec, edge case 7): two roles with the same name under
// different IAM paths reduce to the same key and match each other. Accepted and
// documented rather than silently assumed impossible — making matching
// path-aware is a spec revision, not a patch.
func principalKey(arn string) string {
	// arn:partition:service:region:account:resource — the resource may itself
	// contain slashes but not colons, for the IAM types matched below.
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 || parts[0] != "arn" {
		return ""
	}
	account, resource := parts[4], parts[5]
	if account == "" {
		return ""
	}
	kind, rest, ok := strings.Cut(resource, "/")
	if !ok || rest == "" {
		return ""
	}

	var name string
	switch kind {
	case "assumed-role":
		// First segment: the rest is the session name, which is per-login and
		// must not participate in the comparison.
		name, _, _ = strings.Cut(rest, "/")
	case "role", "user":
		// Last segment: everything before it is the IAM path.
		name = rest[strings.LastIndex(rest, "/")+1:]
	default:
		// federated-user, root, service roles — not principals an access entry
		// names in the shapes we support. Unknown beats a guess.
		return ""
	}
	if name == "" {
		return ""
	}
	return account + "/" + name
}
