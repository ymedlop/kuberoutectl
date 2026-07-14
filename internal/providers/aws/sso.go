package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// SSO discovery for AWS IAM Identity Center (Entra-federated setups).
//
// kuberoutectl does not implement the SSO OIDC login itself — that stays with
// `aws sso login` (which drives the browser / Entra). Once a token is cached,
// this enumerates every account you can access via the SSO portal APIs and
// writes a namespaced `[profile kr-…]` block per account into ~/.aws/config, so
// the normal per-profile discovery (`sync aws`) then finds clusters across all
// of them — and the profiles also work with plain `aws`/`kubectl`.

// ErrSSOLoginRequired signals there is no usable (unexpired) SSO token for the
// session, so the caller should tell the user to run `aws sso login`.
var ErrSSOLoginRequired = errors.New("no valid SSO token")

// SSOPopulateOptions configures profile population.
type SSOPopulateOptions struct {
	SessionName   string // the [sso-session <name>] block in the config
	PreferredRole string // role to prefer per account; empty = auto
	Region        string // region set on generated profiles; empty = sso_region
	ConfigPath    string // path to ~/.aws/config
	CacheDir      string // path to ~/.aws/sso/cache
	Progress      providers.Progress
}

// SSOPopulateResult reports what population did.
type SSOPopulateResult struct {
	SessionName string   `json:"session"`
	Accounts    int      `json:"accounts"`
	Written     []string `json:"written"`
	Skipped     []string `json:"skipped"`
}

type ssoSession struct{ StartURL, Region string }
type ssoToken struct {
	AccessToken string
	Region      string
	ExpiresAt   time.Time
}
type ssoAccount struct{ AccountID, AccountName string }

// PopulateSSOProfiles enumerates accounts/roles for an SSO session and writes
// missing profiles into the config. It never modifies existing entries; it only
// appends `[profile kr-…]` blocks that are not already present (idempotent).
func (p *Provider) PopulateSSOProfiles(ctx context.Context, opts SSOPopulateOptions) (SSOPopulateResult, error) {
	prog := providers.ProgressOr(opts.Progress)

	awsBin, err := p.resolver.Resolve(BinaryName)
	if err != nil {
		return SSOPopulateResult{}, err
	}

	session, err := readSSOSession(opts.ConfigPath, opts.SessionName)
	if err != nil {
		return SSOPopulateResult{}, err
	}

	prog.Step("looking for a valid SSO token")
	token, err := findSSOToken(opts.CacheDir, session.StartURL, p.now())
	if err != nil {
		if errors.Is(err, ErrSSOLoginRequired) {
			return SSOPopulateResult{}, fmt.Errorf("not signed in to SSO — run `aws sso login --sso-session %s`", opts.SessionName)
		}
		return SSOPopulateResult{}, err
	}

	profileRegion := opts.Region
	if profileRegion == "" {
		profileRegion = session.Region
	}

	prog.Step("listing accounts (aws sso list-accounts)")
	accountsOut, _, err := p.runner.Run(ctx, awsBin, "sso", "list-accounts",
		"--access-token", token.AccessToken, "--region", session.Region, "--output", "json")
	if err != nil {
		return SSOPopulateResult{}, fmt.Errorf("aws sso list-accounts failed (token may be expired — run `aws sso login`): %w", err)
	}
	accounts, err := parseSSOAccounts(accountsOut)
	if err != nil {
		return SSOPopulateResult{}, err
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].AccountID < accounts[j].AccountID })

	existing, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		return SSOPopulateResult{}, fmt.Errorf("read %s: %w", opts.ConfigPath, err)
	}
	existingContent := string(existing)

	res := SSOPopulateResult{SessionName: opts.SessionName, Accounts: len(accounts)}
	var toAppend strings.Builder

	for i, acc := range accounts {
		label := acc.AccountName
		if label == "" {
			label = acc.AccountID
		}
		prog.Step("account %s (%d/%d)", label, i+1, len(accounts))

		rolesOut, _, rErr := p.runner.Run(ctx, awsBin, "sso", "list-account-roles",
			"--account-id", acc.AccountID, "--access-token", token.AccessToken,
			"--region", session.Region, "--output", "json")
		if rErr != nil {
			continue // account we can't enumerate roles for — skip, not fatal
		}
		roles, pErr := parseSSORoles(rolesOut)
		if pErr != nil {
			continue
		}
		role, ok := pickRole(roles, opts.PreferredRole)
		if !ok {
			continue // no roles in this account
		}

		name := profileName(acc, role)
		if profileExists(existingContent, name) || profileExists(toAppend.String(), name) {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		toAppend.WriteString(buildProfileBlock(name, opts.SessionName, acc.AccountID, role, profileRegion))
		res.Written = append(res.Written, name)
	}

	if toAppend.Len() > 0 {
		if err := appendToFile(opts.ConfigPath, toAppend.String()); err != nil {
			return SSOPopulateResult{}, err
		}
	}
	prog.Step("wrote %d profile(s)", len(res.Written))
	return res, nil
}

// --- config parsing ---

func readSSOSession(path, name string) (ssoSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ssoSession{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parseSSOSession(string(data), name)
}

// parseSSOSession extracts sso_start_url/sso_region from a [sso-session <name>]
// block. It is a minimal INI scan — enough to read one section — deliberately
// avoiding a full INI dependency.
func parseSSOSession(content, name string) (ssoSession, error) {
	header := "[sso-session " + name + "]"
	var s ssoSession
	inSection := false
	for _, ln := range strings.Split(content, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inSection = t == header
			continue
		}
		if !inSection {
			continue
		}
		if k, v, ok := cutKV(t); ok {
			switch k {
			case "sso_start_url":
				s.StartURL = v
			case "sso_region":
				s.Region = v
			}
		}
	}
	if s.StartURL == "" {
		return ssoSession{}, fmt.Errorf("no [sso-session %s] with sso_start_url found; create one or run `aws configure sso`", name)
	}
	if s.Region == "" {
		return ssoSession{}, fmt.Errorf("[sso-session %s] is missing sso_region", name)
	}
	return s, nil
}

func cutKV(line string) (key, value string, ok bool) {
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

// --- token cache ---

// findSSOToken scans the SSO cache directory for a token whose startUrl matches
// the session and whose expiry is in the future. Matching by startUrl (rather
// than recomputing the cache-key hash) is robust across aws CLI versions.
func findSSOToken(cacheDir, startURL string, now time.Time) (ssoToken, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return ssoToken{}, ErrSSOLoginRequired // no cache dir => not logged in
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if err != nil {
			continue
		}
		var raw struct {
			StartURL    string `json:"startUrl"`
			AccessToken string `json:"accessToken"`
			Region      string `json:"region"`
			ExpiresAt   string `json:"expiresAt"`
		}
		if json.Unmarshal(data, &raw) != nil {
			continue
		}
		if raw.StartURL != startURL || raw.AccessToken == "" {
			continue
		}
		exp, perr := time.Parse(time.RFC3339, raw.ExpiresAt)
		if perr != nil || !exp.After(now) {
			continue // unparseable or expired
		}
		return ssoToken{AccessToken: raw.AccessToken, Region: raw.Region, ExpiresAt: exp}, nil
	}
	return ssoToken{}, ErrSSOLoginRequired
}

// --- portal API parsing ---

func parseSSOAccounts(data []byte) ([]ssoAccount, error) {
	var r struct {
		AccountList []struct {
			AccountID   string `json:"accountId"`
			AccountName string `json:"accountName"`
		} `json:"accountList"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode sso list-accounts: %w", err)
	}
	out := make([]ssoAccount, 0, len(r.AccountList))
	for _, a := range r.AccountList {
		out = append(out, ssoAccount{AccountID: a.AccountID, AccountName: a.AccountName})
	}
	return out, nil
}

func parseSSORoles(data []byte) ([]string, error) {
	var r struct {
		RoleList []struct {
			RoleName string `json:"roleName"`
		} `json:"roleList"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode sso list-account-roles: %w", err)
	}
	out := make([]string, 0, len(r.RoleList))
	for _, role := range r.RoleList {
		out = append(out, role.RoleName)
	}
	return out, nil
}

// preferredRoleDefaults are tried, in order, when no explicit role is requested.
var preferredRoleDefaults = []string{"AdministratorAccess"}

// pickRole selects one role per account: the explicitly preferred one if
// present, else a well-known admin role, else the first alphabetically.
func pickRole(roles []string, preferred string) (string, bool) {
	if len(roles) == 0 {
		return "", false
	}
	sorted := append([]string(nil), roles...)
	sort.Strings(sorted)
	if preferred != "" {
		for _, r := range sorted {
			if r == preferred {
				return r, true
			}
		}
	}
	for _, pref := range preferredRoleDefaults {
		for _, r := range sorted {
			if r == pref {
				return r, true
			}
		}
	}
	return sorted[0], true
}

// --- profile generation ---

var nonProfileChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

func sanitize(s string) string {
	return strings.Trim(nonProfileChars.ReplaceAllString(s, "-"), "-")
}

func profileName(acc ssoAccount, role string) string {
	base := acc.AccountName
	if base == "" {
		base = acc.AccountID
	}
	return "kr-" + sanitize(base) + "-" + sanitize(role)
}

func profileExists(configContent, name string) bool {
	return strings.Contains(configContent, "[profile "+name+"]")
}

func buildProfileBlock(name, session, accountID, role, region string) string {
	var b strings.Builder
	b.WriteString("\n[profile " + name + "]\n")
	b.WriteString("sso_session = " + session + "\n")
	b.WriteString("sso_account_id = " + accountID + "\n")
	b.WriteString("sso_role_name = " + role + "\n")
	if region != "" {
		b.WriteString("region = " + region + "\n")
	}
	return b.String()
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s for append: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("append to %s: %w", path, err)
	}
	return nil
}
