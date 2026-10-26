package doctor

import (
	"context"
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
)

func (s Service) serviceChecks(ctx context.Context) []Check {
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	supervisor := service.Service{Host: s.Host, Config: s.ServiceConfig}
	report, err := supervisor.Status(bounded)
	fix := "run kitbag service install; inspect kitbag service status"
	if err != nil {
		return []Check{result("service", "cannot inspect user service", fix, err, Warn)}
	}
	if !report.Installed {
		return []Check{{ID: "service", Status: OK, Message: "background service is not installed (optional)"}}
	}
	message := ""
	if report.BinaryMissing {
		message = fmt.Sprintf("service executable %s no longer exists (removed by a package upgrade or uninstall?); ", report.Binary)
		fix = "run kitbag service install with the installed kitbag to point the service at it"
	}
	message += fmt.Sprintf("loaded=%t active=%t enabled=%t stale=%t last_run=%s", report.Loaded, report.Active, report.Enabled, report.Stale, report.LastRun)
	if report.ExitCode != nil {
		message += fmt.Sprintf(" exit_code=%d", *report.ExitCode)
		if *report.ExitCode == service.RestartExitCode {
			message += " (restarted after its executable was replaced)"
		}
	}
	status := OK
	if unhealthyService(report) {
		status = Warn
	} else {
		fix = ""
	}
	const serviceChecks = 2
	checks := make([]Check, 1, serviceChecks)
	checks[0] = Check{ID: "service", Status: status, Message: message, Fix: fix}
	return append(checks, s.pollChecks(report)...)
}

// unhealthyService accepts the daemon's restart status: launchd keeps
// reporting it as the last exit code while the restarted daemon runs.
func unhealthyService(report service.Report) bool {
	failed := report.ExitCode != nil && *report.ExitCode != 0 && *report.ExitCode != service.RestartExitCode
	return report.BinaryMissing || !report.Loaded || !report.Active || !report.Enabled || report.Stale || failed
}

func (s Service) pollCheck() Check {
	check := Check{ID: "daemon-poll", Status: OK, Message: "last check is recent"}
	record, _, err := s.Store.LoadCheck()
	switch {
	case err != nil:
		check.Status, check.Message = Warn, "last check record is unavailable"
	case record == nil || record.Repository != s.Repository:
		check.Message = "no check recorded for this repository yet"
	case record.Stale(s.Now()):
		check.Status, check.Message = Warn, "daemon is not checking; last check is older than three poll intervals"
	}
	if check.Status == Warn {
		check.Fix = "inspect kitbag service status and daemon logs; restart the user service"
	}
	return check
}

func (s Service) pollChecks(report service.Report) []Check {
	if report.Mode == "daemon" && report.Active && s.Now != nil && s.Store != nil {
		return []Check{s.pollCheck()}
	}
	return nil
}
