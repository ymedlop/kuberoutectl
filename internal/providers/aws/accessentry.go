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

// listAccessEntries fetches every page of a cluster's entry list through one
// profile. The profile only has to be able to *list* — that is an IAM permission
// on the cluster, not a statement about who is admitted — so one call answers for
// every profile that reaches it.
//
// The two failure kinds are returned separately on purpose. A command failure is
// ok=false with a reason, which every caller treats as resilient. A parse failure
// is an error, and the caller decides: fatal during discovery, degraded to a
// single unusable answer during a live check on one cluster. Collapsing them here
// would force one of those two callers to be wrong.
func (p *Provider) listAccessEntries(ctx context.Context, awsBin, cluster, region, profile string) (entries []string, ok bool, reason string, err error) {
	args := []string{"eks", "list-access-entries", "--cluster-name", cluster, "--profile", profile, "--region", region, "--output", "json"}
	for token := ""; ; {
		call := args
		if token != "" {
			call = append(append([]string{}, args...), "--starting-token", token)
		}
		out, _, rErr := p.runner.Run(ctx, awsBin, call...)
		if rErr != nil {
			return nil, false, "profile " + profile + " may lack eks:ListAccessEntries on this cluster", nil
		}
		page, pErr := parseAccessEntries(out)
		if pErr != nil {
			return nil, false, "", pErr
		}
		entries = append(entries, page.AccessEntries...)
		if page.NextToken == "" {
			return entries, true, "", nil
		}
		token = page.NextToken
	}
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

	entries, ok, _, pErr := p.listAccessEntries(ctx, awsBin, cluster, region, profile)
	if pErr != nil {
		// A parse failure during discovery is fatal, per #112: the answer being
		// skipped is itself an authorization claim, and degrading it to an empty
		// list would report across the fleet that nobody has access.
		return accessResult{}, fmt.Errorf("aws: parse access entries for cluster %q: %w", cluster, pErr)
	}
	if !ok {
		prog.Step("could not list access entries for %q (profile %q may lack eks:ListAccessEntries) — every profile stays unknown", cluster, profile)
		return accessResult{check: domain.AccessCheckUnavailable}, nil
	}

	// Scoped to this cluster's own candidates: keys holds every profile
	// discovered, and matching the rest would compute verdicts for profiles that
	// have nothing to do with this cluster.
	groupKeys := make(map[domain.CredentialID]string, len(group))
	for _, t := range group {
		groupKeys[t.CredentialID] = keys[t.CredentialID]
	}
	return accessResult{check: mode, operable: matchOperable(entries, groupKeys)}, nil
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

// CheckAccess implements providers.AccessChecker: one live lookup for one
// cluster, answering for every credential that reaches it.
//
// It **never** returns an error for a command or parse failure, which is the
// opposite of the discovery path and deliberately so. During `sync` a malformed
// response can poison a whole inventory, so it is fatal. Here the blast radius
// is one line of output on one command, and refusing to activate a cluster
// because the EKS API hiccuped would lock the operator out at exactly the moment
// they are trying to diagnose it. The error return is reserved for a caller
// mistake — a target this provider does not own.
//
// The parse failure still has to be *distinguishable*. Its reason names a
// possible CLI format change, while the routine cases say only that nothing
// could be told; a format regression phrased like a CONFIG_MAP cluster would
// hide behind the common case forever, which is the failure the provider error
// convention exists to prevent.
func (p *Provider) CheckAccess(ctx context.Context, t domain.Target, creds []domain.Credential) (providers.AccessCheck, error) {
	if t.ProviderID != ProviderID {
		return providers.AccessCheck{}, fmt.Errorf("aws: target %q belongs to provider %q", t.ID, t.ProviderID)
	}
	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return providers.AccessCheck{Reason: "the aws CLI could not be resolved"}, nil
	}

	mode, ok := accessCheckMode(t.Metadata["authentication_mode"])
	switch {
	case !ok:
		// Most likely a target cached before the mode was recorded. Naming the
		// remedy matters: a sync fixes it, and nothing else will.
		return providers.AccessCheck{Reason: "this target predates the access check — run `kuberoutectl sync aws`"}, nil
	case mode == domain.AccessCheckConfigMap:
		return providers.AccessCheck{Mode: mode, Reason: "the cluster uses CONFIG_MAP authentication, where access entries do not apply"}, nil
	}

	profile := t.Metadata["profile"]
	entries, listed, reason, pErr := p.listAccessEntries(ctx, awsBin, t.Name, t.Region, profile)
	if pErr != nil {
		return providers.AccessCheck{
			Mode:   domain.AccessCheckUnavailable,
			Reason: "the access-entry response could not be read (" + pErr.Error() + ") — possible aws CLI format change",
		}, nil
	}
	if !listed {
		return providers.AccessCheck{Mode: domain.AccessCheckUnavailable, Reason: reason}, nil
	}

	keys := make(map[domain.CredentialID]string, len(creds))
	for _, c := range creds {
		keys[c.ID] = principalKey(c.Identity)
	}
	operable := matchOperable(entries, keys)

	// Built in the caller's credential order so repeated calls do not churn.
	out := providers.AccessCheck{Mode: mode}
	for _, c := range creds {
		if operable[c.ID] {
			out.Operable = append(out.Operable, c.ID)
		}
	}
	return out, nil
}
