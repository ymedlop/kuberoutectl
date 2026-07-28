package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
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

// accessCheckMode maps the raw authenticationMode from describe-cluster onto
// what a check could establish. An unrecognised or absent mode yields ok=false:
// the mode decides whether *absence* is authoritative, and guessing wrong there
// turns "we could not see" into "you will be refused".
func accessCheckMode(raw string) (domain.AccessCheckMode, bool) {
	switch raw {
	case "API":
		return domain.AccessCheckAPI, true
	case "API_AND_CONFIG_MAP":
		return domain.AccessCheckAPIAndConfigMap, true
	case "CONFIG_MAP":
		return domain.AccessCheckConfigMap, true
	default:
		return "", false
	}
}

// matchOperable reduces every entry to a principal key and reports which of the
// given credentials are among them.
//
// Empty keys on either side are skipped. A credential whose STS identity had no
// ARN reduces to "", and so does a malformed entry — matching those to each
// other would report a broken identity as admitted everywhere.
func matchOperable(entries []string, keys map[domain.CredentialID]string) map[domain.CredentialID]bool {
	listed := map[string]bool{}
	for _, e := range entries {
		if k := principalKey(e); k != "" {
			listed[k] = true
		}
	}
	out := map[domain.CredentialID]bool{}
	for id, key := range keys {
		if key != "" && listed[key] {
			out[id] = true
		}
	}
	return out
}

// checkAccessEntries answers, for one cluster reached by several profiles, which
// of them hold an EKS access entry.
//
// One call covers the whole cluster — the entry list names every principal, so
// asking once and matching locally is both cheaper and more complete than
// asking per profile. The call is made through the group's first profile;
// listing entries is an IAM permission on the cluster, not a statement about
// who is admitted.
//
// Failure is resilient (the sync completes with an `unavailable` verdict), but a
// parse failure on a *successful* call is a hard error. That asymmetry matters
// more here than anywhere else in this adapter: the answer being skipped is
// itself an authorization claim, so degrading it to an empty list would report
// that nobody has access.
func (p *Provider) checkAccessEntries(ctx context.Context, awsBin string, group []domain.Target, keys map[domain.CredentialID]string, prog providers.Progress) (accessResult, error) {
	cluster := group[0].Name
	mode, ok := accessCheckMode(group[0].Metadata["authentication_mode"])
	switch {
	case !ok:
		prog.Step("cannot read the authentication mode of %q — not checking access entries", cluster)
		return accessResult{}, nil
	case mode == domain.AccessCheckConfigMap:
		prog.Step("%q uses CONFIG_MAP authentication — access entries do not apply", cluster)
		return accessResult{check: mode}, nil
	}

	profile, region := group[0].Metadata["profile"], group[0].Region
	prog.Step("checking access entries for %q (%d profiles)", cluster, len(group))

	args := []string{"eks", "list-access-entries", "--cluster-name", cluster, "--profile", profile, "--region", region, "--output", "json"}
	var entries []string
	for token := ""; ; {
		call := args
		if token != "" {
			call = append(append([]string{}, args...), "--starting-token", token)
		}
		out, _, err := p.runner.Run(ctx, awsBin, call...)
		if err != nil {
			// Command failure: resilient, per the provider error convention. Name
			// the permission — otherwise the operator sees a fleet of `unknown`
			// with nothing to act on.
			prog.Step("could not list access entries for %q (profile %q may lack eks:ListAccessEntries) — every profile stays unknown", cluster, profile)
			return accessResult{check: domain.AccessCheckUnavailable}, nil
		}
		page, pErr := parseAccessEntries(out)
		if pErr != nil {
			return accessResult{}, fmt.Errorf("aws: parse access entries for cluster %q: %w", cluster, pErr)
		}
		entries = append(entries, page.AccessEntries...)
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}

	return accessResult{check: mode, operable: matchOperable(entries, keys)}, nil
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
