package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
)

func (c *commandLine) serviceRefreshCommand() *cobra.Command {
	var asJSON, dryRun bool
	cmd := &cobra.Command{Use: "refresh", Short: "Repair an installed service after an upgrade", Long: "Compare the installed definition with this binary's generator, preserving saved installation inputs. Atomically replace stale definitions and reload/restart only as needed, including a running daemon with a different version. An uninstalled service is left alone. --dry-run reports proposed work without writing or contacting the supervisor to change it. Exit 0 on success (including no-op), 1 on failure, 2 for invalid input, 5 on unsupported systems.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := c.refreshService(cmd.Context(), dryRun)
		if err != nil {
			return classified(err)
		}
		if asJSON {
			return classified(json.NewEncoder(c.stdout).Encode(r))
		}
		message := "Service is up to date."
		switch {
		case !r.Installed:
			message = "Service is not installed; nothing to refresh."
		case r.DryRun && r.Changed:
			message = "Service would be refreshed."
		case r.RestartNeeded && r.DaemonVersion != "" && r.DaemonVersion != r.BinaryVersion:
			message = fmt.Sprintf("Restarted the running daemon (%s -> %s)", r.DaemonVersion, r.BinaryVersion)
		case r.Changed:
			message = "Service refreshed."
		}
		_, err = fmt.Fprintln(c.stdout, message)
		return classified(err)
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable service_schema=1 JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report changes without writing or restarting")
	return cmd
}

func (c *commandLine) refreshService(ctx context.Context, dryRun bool) (service.RefreshResult, error) {
	request, err := c.serviceVersion(ctx, dryRun)
	if err != nil {
		return service.RefreshResult{}, err
	}
	return c.userService().Refresh(ctx, request)
}

func (c *commandLine) serviceVersion(ctx context.Context, dryRun bool) (service.RefreshRequest, error) {
	installed, err := c.userService().Installed()
	request := service.RefreshRequest{BinaryVersion: toolVersion(), DryRun: dryRun}
	if err != nil || !installed {
		return request, err
	}
	request.DaemonVersion, request.Running, err = (client.Client{Host: c.host}).DaemonVersion(ctx)
	return request, err
}
