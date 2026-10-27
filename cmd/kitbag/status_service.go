package main

import (
	"context"
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

func (c *commandLine) statusService(ctx context.Context, status *workflow.Status) {
	daemonVersion, running, err := (client.Client{Host: c.host}).DaemonVersion(ctx)
	if err != nil {
		status.Warnings = append(status.Warnings, "Daemon version unavailable; inspect daemon logs.")
	} else if running {
		status.DaemonRunning, status.DaemonVersion = true, daemonVersion
		if daemonVersion != status.BinaryVersion {
			status.Warnings = append(status.Warnings, fmt.Sprintf("Version mismatch: daemon %s, binary %s; run kitbag service refresh.", daemonVersion, status.BinaryVersion))
		}
	}
	report, err := c.userService().Status(ctx)
	if err == nil && report.Installed && report.Stale {
		status.ServiceStale = true
		status.Warnings = append(status.Warnings, "Service definition is stale; run kitbag service refresh.")
	}
}
