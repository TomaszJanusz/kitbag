package doctor

import (
	"context"
	"fmt"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
)

func (s Service) serviceChecks(ctx context.Context) []Check {
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	versions := s.daemonVersionCheck(bounded)
	return append(s.supervisorChecks(bounded), versions...)
}

func (s Service) supervisorChecks(ctx context.Context) []Check {
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	supervisor := service.Service{Host: s.Host, Config: s.ServiceConfig}
	report, err := supervisor.Status(bounded)
	fix := "run kitbag service refresh; inspect kitbag service status"
	if err != nil {
		return []Check{result("service", "cannot inspect user service", fix, err, Warn)}
	}
	if !report.Installed {
		// KitBag is the CLI, the TUI and the background service; without the
		// service this machine syncs only when someone runs kitbag sync.
		return []Check{{ID: "service", Status: Warn, Message: "background service is not installed; this machine syncs only when you run kitbag sync", Fix: "run kitbag service install"}}
	}
	message := ""
	if report.BinaryMissing {
		message = fmt.Sprintf("service executable %s no longer exists (removed by a package upgrade or uninstall?); ", report.Binary)
		fix = "run kitbag service refresh with the installed kitbag to point the service at it"
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

func (s Service) daemonVersionCheck(ctx context.Context) []Check {
	if s.DaemonVersion == nil {
		return nil
	}
	version, running, err := s.DaemonVersion(ctx)
	if err != nil {
		return []Check{{ID: "daemon-version", Status: Warn, Message: "cannot read daemon version", Fix: "inspect kitbag status and daemon logs"}}
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	pending, recoveryErr := (service.Service{Host: s.Host, Config: s.ServiceConfig}).RecoverVersion(ctx, service.RefreshRequest{BinaryVersion: s.Version, DaemonVersion: version, Running: running}, now)
	if recoveryErr != nil {
		return []Check{{ID: "daemon-version", Status: Warn, Message: fmt.Sprintf("automatic service recovery failed: %v", recoveryErr), Fix: "inspect daemon logs and kitbag service status"}}
	}
	if pending {
		return []Check{{ID: "daemon-version", Status: OK, Message: fmt.Sprintf("daemon %s, binary %s; restarting on the new version, usually within a few minutes", version, s.Version)}}
	}
	if !running {
		return nil
	}
	check := Check{ID: "daemon-version", Status: OK, Message: fmt.Sprintf("daemon %s, binary %s", version, s.Version)}
	if version != s.Version {
		check.Status, check.Fix = Warn, "run kitbag service refresh"
		check.Message += "; version mismatch"
	}
	return []Check{check}
}
