package cli

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
	"github.com/ymedlop/kuberoutectl/internal/services"
	"github.com/ymedlop/kuberoutectl/internal/updatecheck"
)

// fakeChecker returns a canned verdict and records that it was asked.
type fakeChecker struct {
	res   updatecheck.Result
	calls int
}

func (f *fakeChecker) Evaluate(_ context.Context, current string) updatecheck.Result {
	f.calls++
	r := f.res
	r.Current = current
	return r
}

// outdated, current and failed build the three verdicts a checker can return.
func outdated(latest string) *fakeChecker {
	return &fakeChecker{res: updatecheck.Result{Latest: latest, Verdict: updatecheck.VerdictOutdated}}
}
func upToDate(latest string) *fakeChecker {
	return &fakeChecker{res: updatecheck.Result{Latest: latest, Verdict: updatecheck.VerdictCurrent}}
}
func failed(reason string) *fakeChecker {
	return &fakeChecker{res: updatecheck.Result{Verdict: updatecheck.VerdictUnknown, Reason: reason}}
}

// neverChecker fails the test if anything asks it for a release. It is how the
// "no request is made" guarantees are asserted — a result-level check would pass
// just as well when the call happened and its answer was discarded.
type neverChecker struct{ t *testing.T }

func (n neverChecker) Evaluate(context.Context, string) updatecheck.Result {
	n.t.Helper()
	n.t.Error("an update request was made when none was allowed")
	return updatecheck.Result{}
}

func TestUpdateCheckRow(t *testing.T) {
	cases := []struct {
		name       string
		current    string
		checker    *fakeChecker
		wantStatus services.CheckStatus
		wantIn     []string
		wantNotIn  []string
	}{
		{
			name:       "a newer release is a warning naming both versions",
			current:    "1.0.0",
			checker:    outdated("v1.2.0"),
			wantStatus: services.CheckWarn,
			wantIn:     []string{"1.2.0", "1.0.0", updatecheck.ReleasesURL},
		},
		{
			name:       "up to date is ok and says nothing about upgrading",
			current:    "1.2.0",
			checker:    upToDate("v1.2.0"),
			wantStatus: services.CheckOK,
			wantNotIn:  []string{updatecheck.ReleasesURL, "available"},
		},
		{
			name:       "ahead of the latest release is ok, not a warning",
			current:    "1.3.0",
			checker:    upToDate("v1.2.0"),
			wantStatus: services.CheckOK,
			wantNotIn:  []string{"available"},
		},
		{
			// The row stays and carries the reason. A check that disappears when it
			// fails is indistinguishable from one that was never wired up — the
			// silent-skip bug fixed in #111, in a different costume.
			name:       "an unreachable API is reported, not dropped",
			current:    "1.0.0",
			checker:    failed("could not reach the releases API"),
			wantStatus: services.CheckOK,
			wantIn:     []string{"could not reach the releases API"},
			wantNotIn:  []string{"available"},
		},
		{
			// The most dangerous confusion available here: a failed check must never
			// read as a clean bill of health.
			name:       "a failed check never claims you are up to date",
			current:    "1.0.0",
			checker:    failed("the releases API rate-limited this address"),
			wantStatus: services.CheckOK,
			wantNotIn:  []string{"latest release", "up to date"},
		},
		{
			// Enabled() should have prevented this, but the row builder must not
			// depend on its caller to be correct about it.
			name:       "an uncomparable running version yields no verdict",
			current:    "0.0.0-snapshot-abc1234",
			checker:    failed("this build has no comparable version"),
			wantStatus: services.CheckOK,
			wantNotIn:  []string{"available", updatecheck.ReleasesURL},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := updateCheckRow(context.Background(), tc.current, tc.checker)
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q (detail: %q)", got.Status, tc.wantStatus, got.Detail)
			}
			if got.Name != "version" {
				t.Errorf("name = %q, want %q", got.Name, "version")
			}
			if got.Detail == "" {
				t.Error("detail is empty; the rendered row would say nothing")
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(got.Detail, want) {
					t.Errorf("detail %q does not contain %q", got.Detail, want)
				}
			}
			for _, unwanted := range tc.wantNotIn {
				if strings.Contains(got.Detail, unwanted) {
					t.Errorf("detail %q must not contain %q", got.Detail, unwanted)
				}
			}
		})
	}
}

// The opt-out has to stop the request, not just the output. Enabled is consulted
// before any client is built, which is the ordering that makes "no row" and "no
// request" one decision instead of two.
//
// This asserts the predicate only. The end-to-end guarantee — that doctorCmd
// really never reaches the checker — is TestDoctor_RowAppearsOnlyWhenChecked,
// which drives the command with a checker that fails on use. An earlier version
// of this test tried to do both and guarded the second half with the same
// `Enabled(...)` call it had just proven false, making that half unreachable: a
// guard repeating verbatim the condition it is supposed to protect is dead code,
// not defence in depth.
func TestUpdateCheck_OptOutDisablesThePredicate(t *testing.T) {
	t.Setenv(updatecheck.EnvDisable, "1")
	if updatecheck.Enabled("1.0.0") {
		t.Fatal("the opt-out did not disable the check")
	}
}

// doctorApp is testApp with one provider registered, so `doctor` produces real
// rows and the presence or absence of the version row is visible against them.
func doctorApp(t *testing.T, version string, c releaseChecker) *app {
	t.Helper()
	a := testApp(t)
	if err := a.registry.Register(stubProvider{id: "azure"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	a.requiredBinary = map[string]string{"azure": "az"}
	a.resolver = stubResolver{}
	a.version, a.checker = version, c
	return a
}

// The row appears when a check ran, and the command is otherwise exactly what it
// was before — so an installation that opted out, or a dev build, sees no
// change at all.
func TestDoctor_RowAppearsOnlyWhenChecked(t *testing.T) {
	with, err := runCmd(doctorApp(t, "1.0.0", outdated("v1.2.0")).doctorCmd(), "")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(with, "1.2.0 is available") {
		t.Errorf("expected the update row, got:\n%s", with)
	}
	if !strings.Contains(with, "provider:azure") {
		t.Errorf("the provider rows must still render, got:\n%s", with)
	}

	// Opted out: no row, and — the assertion that matters — the checker is never
	// reached, so no request could have been made.
	t.Setenv(updatecheck.EnvDisable, "1")
	without, err := runCmd(doctorApp(t, "1.0.0", neverChecker{t}).doctorCmd(), "")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if strings.Contains(without, "available") || strings.Contains(without, "version") {
		t.Errorf("a suppressed check must add no row, got:\n%s", without)
	}
	if !strings.Contains(without, "provider:azure") {
		t.Errorf("the provider rows must still render, got:\n%s", without)
	}
}

// A dev or snapshot build has no comparable version, so it must not check —
// which is also what keeps `go run ./...` and the rolling snapshot from making
// requests during development.
func TestDoctor_NonReleaseBuildNeverChecks(t *testing.T) {
	for _, version := range []string{"dev", "0.0.0-snapshot-abc1234"} {
		out, err := runCmd(doctorApp(t, version, neverChecker{t}).doctorCmd(), "")
		if err != nil {
			t.Fatalf("doctor (%s): %v", version, err)
		}
		if strings.Contains(out, "available") {
			t.Errorf("%s build produced an update row:\n%s", version, out)
		}
	}
}

// The update check row must be appended, never inserted. `doctor -o json` is
// documented as machine-readable, so an existing consumer indexing the array
// must not find its provider rows shifted underneath it.
func TestDoctor_UpdateRowIsAppendedLast(t *testing.T) {
	checks := []services.Check{{Name: "provider:aws"}, {Name: "provider:azure"}}
	got := append(checks, updateCheckRow(context.Background(), "1.0.0", upToDate("v1.0.0")))

	if got[0].Name != "provider:aws" || got[1].Name != "provider:azure" {
		t.Errorf("provider rows moved: %+v", got)
	}
	if got[len(got)-1].Name != "version" {
		t.Errorf("the version row is not last: %+v", got)
	}
}

// The outbound boundary is a design decision, so it is enforced rather than
// documented. Only internal/cli may reach internal/updatecheck: if services,
// mcpserver, providers or domain ever import it, a command the user ran for
// another purpose could acquire a network call without anyone noticing.
func TestOnlyCLIImportsUpdatecheck(t *testing.T) {
	const pkg = "github.com/ymedlop/kuberoutectl/internal/updatecheck"
	root := filepath.Join("..", "..", "internal")

	inspected := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		dir := filepath.Dir(rel)
		// The package itself, and the CLI that is allowed to wire it.
		if dir == "updatecheck" || dir == "cli" {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		inspected++
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == pkg {
				t.Errorf("%s imports %s; only internal/cli may", rel, pkg)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	// A guard that walked nothing passes silently and proves nothing — the exact
	// failure mode this file exists to prevent elsewhere.
	if inspected < 10 {
		t.Fatalf("only inspected %d files; the guard is not actually looking at the tree", inspected)
	}
}

// stubProvider and stubResolver give doctor something to report on without
// touching a real CLI.
type stubProvider struct{ id domain.ProviderID }

func (s stubProvider) ID() domain.ProviderID             { return s.id }
func (s stubProvider) Capabilities() domain.Capabilities { return domain.Capabilities{} }
func (s stubProvider) Discover(context.Context, providers.DiscoveryInput) (providers.DiscoveryResult, error) {
	return providers.DiscoveryResult{}, nil
}
func (s stubProvider) Renew(context.Context, domain.Credential) error { return nil }

type stubResolver struct{}

func (stubResolver) Resolve(name string) (string, error) { return "/usr/bin/" + name, nil }

// `version` on its own must make no request. This is the guarantee the whole
// design exists to keep, and it is asserted with a checker that fails the test
// on use rather than by inspecting output.
func TestVersion_PlainNeverChecks(t *testing.T) {
	a := doctorApp(t, "1.0.0", neverChecker{t})
	out, err := runCmd(a.versionCmd(), "")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.Contains(out, "available") || strings.Contains(out, "update check") {
		t.Errorf("plain version reported an update:\n%s", out)
	}
}

func TestVersion_CheckUpdateReportsAndStaysQuietWhenCurrent(t *testing.T) {
	a := doctorApp(t, "1.0.0", outdated("v1.2.0"))
	out, err := runCmd(a.versionCmd(), "", "--check-update")
	if err != nil {
		t.Fatalf("version --check-update: %v", err)
	}
	if !strings.Contains(out, "1.2.0 is available") || !strings.Contains(out, updatecheck.ReleasesURL) {
		t.Errorf("expected the update line, got:\n%s", out)
	}

	// The canned verdict is what makes this the up-to-date case: the comparison
	// itself is decided in internal/updatecheck and tested there, so a CLI fake
	// that recomputed it would be testing the fake.
	a = doctorApp(t, "1.2.0", upToDate("v1.2.0"))
	out, err = runCmd(a.versionCmd(), "", "--check-update")
	if err != nil {
		t.Fatalf("version --check-update: %v", err)
	}
	if strings.Contains(out, "available") {
		t.Errorf("an up-to-date build must not offer an upgrade, got:\n%s", out)
	}
}

// The JSON shape is additive: default output is unchanged, and a failed check
// reports its reason instead of pretending there is a verdict.
func TestVersionJSON_UpdateFieldsAreAdditive(t *testing.T) {
	// Decoded fresh every time on purpose: json.Unmarshal MERGES into a non-nil
	// map rather than replacing it, so reusing one across cases carries keys
	// forward and makes later assertions read the previous case's result.
	decode := func(t *testing.T, out string) map[string]any {
		t.Helper()
		got := map[string]any{}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		return got
	}
	run := func(t *testing.T, a *app, args ...string) map[string]any {
		t.Helper()
		a.output = formatJSON
		out, err := runCmd(a.versionCmd(), "", args...)
		if err != nil {
			t.Fatalf("version: %v", err)
		}
		return decode(t, out)
	}

	got := run(t, doctorApp(t, "1.0.0", neverChecker{t}))
	for _, key := range []string{"latest_version", "update_available", "update_check"} {
		if _, ok := got[key]; ok {
			t.Errorf("key %q must be absent without --check-update: %v", key, got)
		}
	}

	got = run(t, doctorApp(t, "1.0.0", outdated("v1.2.0")), "--check-update")
	if got["latest_version"] != "v1.2.0" || got["update_available"] != true {
		t.Errorf("expected structured update fields, got %v", got)
	}
	if _, ok := got["update_check"]; ok {
		t.Errorf("a real verdict must not also carry a skip reason: %v", got)
	}

	// A failed lookup reports the reason and must NOT claim a version — the two
	// are mutually exclusive by construction.
	got = run(t, doctorApp(t, "1.0.0", failed("could not reach the releases API")), "--check-update")
	if _, ok := got["latest_version"]; ok {
		t.Errorf("a failed check must not report a latest_version: %v", got)
	}
	if _, ok := got["update_check"]; !ok {
		t.Errorf("a failed check must report why: %v", got)
	}
}
