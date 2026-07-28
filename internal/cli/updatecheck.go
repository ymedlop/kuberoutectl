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
	Evaluate(ctx context.Context, current string) updatecheck.Result
}

// describeUpdate renders a verdict as the line a user reads. Only the wording
// lives here; which verdict applies is decided in internal/updatecheck, so
// `doctor` and `version --check-update` cannot disagree about when an update is
// worth reporting.
func describeUpdate(r updatecheck.Result) string {
	switch r.Verdict {
	case updatecheck.VerdictOutdated:
		return r.Latest + " is available (you have " + r.Current + ") — " + updatecheck.ReleasesURL
	case updatecheck.VerdictCurrent:
		return r.Current + " is the latest release"
	default:
		return "update check skipped: " + r.Reason
	}
}

// updateCheckRow renders the verdict as a doctor check, so it inherits the
// existing rendering and `-o json` shape rather than needing either to change.
//
// Only an outdated build is a warning. A failed lookup is `ok` with the reason
// attached, because nothing about the user's environment is wrong when GitHub is
// unreachable — but the row must still appear. A check that vanishes on failure
// is indistinguishable from one that was never wired up.
func updateCheckRow(ctx context.Context, current string, c releaseChecker) services.Check {
	res := c.Evaluate(ctx, current)
	status := services.CheckOK
	if res.Verdict == updatecheck.VerdictOutdated {
		status = services.CheckWarn
	}
	return services.Check{
		Name:    "version",
		Status:  status,
		Version: current,
		Detail:  describeUpdate(res),
	}
}
