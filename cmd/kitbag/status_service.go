package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

func (c *commandLine) statusService(ctx context.Context, status *workflow.Status) {
	version, running, versionErr := (client.Client{Host: c.host}).RecoveryDaemonVersion(ctx)
	if versionErr != nil {
		status.Warnings = append(status.Warnings, "Daemon version unavailable; inspect daemon logs.")
	} else if running {
		status.DaemonRunning, status.DaemonVersion = true, version
	}
	pending := c.recoverStatusService(ctx, status, versionErr == nil)
	report, err := c.userService().Status(ctx)
	if err == nil && report.Installed {
		status.ServiceStale = report.Stale
	}
	statusServiceWarnings(status, pending)
}

func (c *commandLine) recoverStatusService(ctx context.Context, status *workflow.Status, observed bool) bool {
	if !observed {
		return false
	}
	pending, err := c.userService().RecoverVersion(ctx, service.RefreshRequest{BinaryVersion: status.BinaryVersion, DaemonVersion: status.DaemonVersion, Running: status.DaemonRunning}, time.Now())
	if err != nil {
		status.Warnings = append(status.Warnings, fmt.Sprintf("Automatic service recovery failed: %v", err))
	}
	return pending
}

func statusServiceWarnings(status *workflow.Status, pending bool) {
	switch {
	case pending:
		status.Warnings = append(status.Warnings, "Service upgrade recovery requested; waiting for the supervisor.")
	case status.DaemonRunning && status.DaemonVersion != status.BinaryVersion:
		status.Warnings = append(status.Warnings, fmt.Sprintf("Version mismatch: daemon %s, binary %s; automatic recovery is unavailable or has not completed; inspect daemon logs.", status.DaemonVersion, status.BinaryVersion))
	}
	if status.ServiceStale && !pending {
		status.Warnings = append(status.Warnings, "Service definition is stale; inspect daemon logs or Repair service.")
	}
}
