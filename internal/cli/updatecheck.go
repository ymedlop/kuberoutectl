package cli

import (
	"context"

	"github.com/ymedlop/kuberoutectl/internal/services"
	"github.com/ymedlop/kuberoutectl/internal/updatecheck"
)

// releaseChecker is the piece of internal/updatecheck the commands consume,
// declared here so tests can supply a checker that fails if it is ever called —
// which is the only way to assert the "no request is made" guarantees.
type releaseChecker interface {
	Latest(ctx context.Context) (version string, ok bool, reason string)
}

// updateStatus is one release lookup, reduced to what both `doctor` and
// `version --check-update` need. Structured rather than a rendered string,
// because `version -o json` reports the latest version as a field and digging it
// back out of prose would be a parser nobody asked for.
type updateStatus struct {
	Current string
	// Latest is set only when a comparison actually happened.
	Latest string
	Newer  bool
	// Detail is always set and always display-ready, including on failure.
	Detail string
}

// checkForUpdate asks for the newest release and decides what can be said.
//
// It lives in the CLI, not in DoctorService, on purpose. Keeping it here means
// the service's documented promise — it "does not attempt discovery or network
// calls" — stays true, and internal/services needs no knowledge of HTTP or of
// release versioning.
//
// Only a *newer* release is worth a warning. Everything else is reported as
// fine-with-a-reason, because nothing about the user's environment is wrong when
// GitHub is unreachable — but it must still be reported. A check that vanishes
// on failure is indistinguishable from one that was never wired up, and calling
// a failed lookup "up to date" would be a confident falsehood built out of
// silence.
func checkForUpdate(ctx context.Context, current string, c releaseChecker) updateStatus {
	st := updateStatus{Current: current}

	latest, ok, reason := c.Latest(ctx)
	if !ok {
		st.Detail = "update check skipped: " + reason
		return st
	}
	newer, comparable := updatecheck.Newer(current, latest)
	if !comparable {
		// Enabled() should have kept us out of here; this must not depend on the
		// caller having been right about that.
		st.Detail = "no update check for this build"
		return st
	}
	st.Latest, st.Newer = latest, newer
	if !newer {
		st.Detail = current + " is the latest release"
		return st
	}
	st.Detail = latest + " is available (you have " + current + ") — " + updatecheck.ReleasesURL
	return st
}

// check renders the status as a doctor row, so it inherits the existing
// rendering and `-o json` shape rather than needing either to change.
func (u updateStatus) check() services.Check {
	status := services.CheckOK
	if u.Newer {
		status = services.CheckWarn
	}
	return services.Check{Name: "version", Status: status, Version: u.Current, Detail: u.Detail}
}

// updateCheckRow is the doctor entry point: look up, then render.
func updateCheckRow(ctx context.Context, current string, c releaseChecker) services.Check {
	return checkForUpdate(ctx, current, c).check()
}
