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
	}
	status := OK
	if unhealthyService(report) {
		status = Warn
	} else {
		fix = ""
	}
	return []Check{{ID: "service", Status: status, Message: message, Fix: fix}}
}

func unhealthyService(report service.Report) bool {
	return report.BinaryMissing || !report.Loaded || !report.Active || !report.Enabled || report.Stale || report.ExitCode != nil && *report.ExitCode != 0
}
