// Package updatecheck answers one question — is there a newer stable release —
// and is the only place in kuberoutectl that makes an outbound network request
// of its own. Everything else delegates external access to a cloud CLI through
// execx.
//
// That boundary is deliberate and enforced by a test: only internal/cli may
// import this package. A tool that handles cloud credentials should be able to
// state exactly which commands talk to the network, and the answer here is two
// diagnostics — `doctor` and `version --check-update` — never an ambient check
// on commands the user ran for another purpose.
//
// Nothing is transmitted: the request is an unauthenticated GET carrying no
// version, no identifier and no usage data.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// EnvDisable suppresses the check entirely when set to any non-empty value.
	EnvDisable = "KUBEROUTECTL_NO_UPDATE_CHECK"
	// ReleasesURL is what the notice points at. The tool does not know how it was
	// installed — brew, scoop, apt, rpm or a downloaded tarball — so it names the
	// version and links here rather than guessing an upgrade command that could
	// leave someone with two conflicting binaries.
	ReleasesURL = "https://github.com/ymedlop/kuberoutectl/releases/latest"

	defaultAPIURL  = "https://api.github.com/repos/ymedlop/kuberoutectl/releases/latest"
	defaultTimeout = 3 * time.Second
)

// Enabled reports whether a check may run at all, given the running build's
// version. It is deliberately consulted *before* any HTTP client is constructed,
// so that "no row is shown" and "no request is made" are the same decision
// rather than two conditionals that can drift apart.
func Enabled(version string) bool {
	if os.Getenv(EnvDisable) != "" {
		return false
	}
	_, ok := parseVersion(version)
	return ok
}

// Newer reports whether latest is a newer stable release than current.
//
// ok is false when either side is not a plain stable release — a dev build, a
// snapshot, a pre-release, or anything that does not parse. In that case there
// is no verdict to give, and callers must say nothing rather than guess: a
// pre-release must never be offered to someone on a stable version, and a dev
// build has no version to compare at all.
func Newer(current, latest string) (newer bool, ok bool) {
	cur, curOK := parseVersion(current)
	lat, latOK := parseVersion(latest)
	if !curOK || !latOK {
		return false, false
	}
	for i := range cur {
		if cur[i] != lat[i] {
			return lat[i] > cur[i], true
		}
	}
	return false, true
}

// parseVersion splits a stable MAJOR.MINOR.PATCH release into its numeric
// fields. An optional leading `v` is accepted because release tags carry it and
// buildinfo.Version does not.
//
// Everything else is rejected rather than coerced: a pre-release or build suffix
// (`-rc.1`, `+build5`), a wrong field count, a non-numeric field, `dev`, and
// `0.0.0-snapshot-<sha>`. Comparing those numerically is where a naive
// implementation either nags forever or panics.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" || strings.ContainsAny(v, "-+") {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Verdict is what a check could establish. Three values, not two: "could not
// tell" is a real answer, and collapsing it into "up to date" would turn a
// network failure into a claim about the user's build.
type Verdict int

const (
	// VerdictUnknown: no comparison happened. Reason says why.
	VerdictUnknown Verdict = iota
	// VerdictCurrent: the running build is the latest stable release, or ahead.
	VerdictCurrent
	// VerdictOutdated: a newer stable release exists.
	VerdictOutdated
)

// Result is one evaluated check. It carries facts, not sentences: the CLI turns
// these into the text a user reads, and `version -o json` reports Latest as a
// field, which it could not do if the answer only existed as prose.
type Result struct {
	Current string
	// Latest is set only when a comparison actually happened.
	Latest  string
	Verdict Verdict
	// Reason is set exactly when Verdict is VerdictUnknown, and is phrased for
	// display.
	Reason string
}

// Evaluate looks up the newest release and decides what can be said about the
// running build.
//
// The decision lives here rather than in the command layer because both `doctor`
// and `version --check-update` need it, and because it is a judgement about
// release state rather than about presentation — the two commands must not be
// able to drift into disagreeing about when an update is worth reporting.
//
// Only a newer release is a positive finding. Every failure yields
// VerdictUnknown with a reason, never VerdictCurrent: reporting an unreachable
// API as "you are up to date" would be a confident falsehood assembled out of
// silence, which is the parse-failure bug of #111 in a different costume.
func (c *Checker) Evaluate(ctx context.Context, current string) Result {
	res := Result{Current: current}

	latest, ok, reason := c.Latest(ctx)
	if !ok {
		res.Reason = reason
		return res
	}
	newer, comparable := Newer(current, latest)
	if !comparable {
		// Enabled should have kept this build out of here; not depending on the
		// caller having been right about that.
		res.Reason = "this build has no comparable version"
		return res
	}
	res.Latest = latest
	if newer {
		res.Verdict = VerdictOutdated
	} else {
		res.Verdict = VerdictCurrent
	}
	return res
}

// release is the subset of the GitHub releases payload we read.
type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
}

// Checker fetches the newest stable release tag.
type Checker struct {
	client *http.Client
	url    string
}

// New builds a Checker. Both arguments are for tests: production passes nils and
// gets the real endpoint with a 3s timeout.
func New(client *http.Client, url string) *Checker {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	if url == "" {
		url = defaultAPIURL
	}
	return &Checker{client: client, url: url}
}

// Latest returns the newest stable release tag.
//
// ok is false for every failure — offline, DNS, timeout, HTTP 403 (GitHub allows
// 60 unauthenticated requests per hour per IP, and a corporate NAT shares one
// address across everyone), any other status, a malformed body, or a payload
// marked prerelease. Each returns a reason meant to be shown, because a check
// that vanishes when it fails is indistinguishable from one that was never
// wired up.
//
// A malformed body in particular must never degrade to "you are up to date":
// that is the update-check form of the parse-failure bug fixed in #111.
func (c *Checker) Latest(ctx context.Context) (version string, ok bool, reason string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return "", false, "could not build the release request"
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", false, "could not reach the releases API"
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return "", false, "the releases API rate-limited this address"
	case resp.StatusCode != http.StatusOK:
		return "", false, fmt.Sprintf("the releases API answered HTTP %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", false, "could not read the releases API response"
	}
	// The `latest` endpoint already excludes pre-releases; checked anyway rather
	// than trusted, because the cost of being wrong is offering an rc to someone
	// running a stable build.
	if rel.Prerelease || rel.TagName == "" {
		return "", false, "no stable release was reported"
	}
	return rel.TagName, true, ""
}
