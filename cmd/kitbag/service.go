package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func (c *commandLine) serviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Manage background synchronization for this user", Long: "Install, uninstall or inspect a systemd user service or macOS LaunchAgent. Configuration, working copies and sync state are kept on uninstall."}
	cmd.AddCommand(c.serviceInstallCommand(), c.serviceUninstallCommand(), c.serviceStatusCommand())
	return cmd
}

func (c *commandLine) userService() service.Service {
	return service.Service{Host: c.host, Config: c.configPath}
}

func (c *commandLine) serviceInstallCommand() *cobra.Command {
	var mode string
	var interval time.Duration
	var noStart, now bool
	cmd := &cobra.Command{Use: "install", Short: "Enable background sync with a daemon or timer", Long: "Write service definitions atomically and enable/start them. Repeating the same installation keeps the running job. --no-start only enables future starts; --now explicitly requests the default immediate start. Timer intervals range from 1m to 24h.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if _, err := c.config.WorkingCopy(c.host); err != nil {
			return classified(err)
		}
		supervisor := c.userService()
		options, err := supervisor.Current(mode, interval)
		if err != nil {
			return classified(err)
		}
		changed, err := supervisor.Install(cmd.Context(), options, noStart)
		if err != nil {
			return classified(err)
		}
		paths, err := supervisor.Paths(options)
		if err != nil {
			return classified(err)
		}
		return classified(c.printInstallation(cmd.Context(), c.stdout, service.Installation{Mode: options.Mode, Paths: paths, Changed: changed}))
	}}
	cmd.Flags().StringVar(&mode, "mode", "daemon", "Service mode: daemon or timer")
	cmd.Flags().DurationVar(&interval, "interval", service.DefaultInterval, "Timer cadence (1m to 24h)")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "Enable the service without starting it now")
	cmd.Flags().BoolVar(&now, "now", false, "Start immediately (the default)")
	cmd.MarkFlagsMutuallyExclusive("now", "no-start")
	return cmd
}

// printInstallation names every file an installation wrote (or found up to
// date), then the systemd linger hint.
func (c *commandLine) printInstallation(ctx context.Context, out io.Writer, installation service.Installation) error {
	message := "Service already installed and up to date"
	if installation.Changed {
		message = "Service installed/updated"
	}
	lines := make([]string, 0, len(installation.Paths)+1)
	lines = append(lines, message+" ("+installation.Mode+" mode):")
	for _, path := range installation.Paths {
		lines = append(lines, "  "+path)
	}
	if _, err := fmt.Fprintln(out, strings.Join(lines, "\n")); err != nil {
		return err
	}
	return c.printLinger(ctx)
}

func (c *commandLine) printLinger(ctx context.Context) error {
	layout, err := c.host.ServiceLayout()
	if err != nil || layout.Manager != "systemd" {
		return err
	}
	enabled, err := c.host.ServiceLinger(ctx)
	if err != nil {
		_, writeErr := fmt.Fprintf(c.stderr, "Could not check linger: %v. For headless operation, check loginctl show-user %s -p Linger.\n", err, layout.User)
		return writeErr
	}
	if enabled {
		return nil
	}
	_, err = fmt.Fprintf(c.stderr, "Linger is disabled. Background sync works in this login session; for headless operation and after logout, enable this user's persistent manager with: loginctl enable-linger %s\n", layout.User)
	return err
}

func (c *commandLine) serviceUninstallCommand() *cobra.Command {
	return &cobra.Command{Use: "uninstall", Short: "Stop background sync and remove service definitions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := c.userService().Uninstall(cmd.Context()); err != nil {
			return classified(err)
		}
		_, err := fmt.Fprintln(c.stdout, "Service uninstalled. Configuration, working copies and sync state were kept.")
		return classified(err)
	}}
}

type serviceReport struct {
	SyncError string            `json:"sync_error,omitempty"`
	LastSync  *state.SyncRecord `json:"last_sync"`
	service.Report
}

func (c *commandLine) serviceStatusCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "status", Short: "Show supervisor health and the last synchronization", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := c.userService().Status(cmd.Context())
		if err != nil {
			return classified(err)
		}
		result := serviceReport{Report: report}
		status, syncErr := c.status(cmd.Context())
		result.LastSync = status.LastSync
		if syncErr != nil {
			result.SyncError = "Sync status unavailable; run kitbag status for details."
		}
		if asJSON {
			return classified(json.NewEncoder(c.stdout).Encode(result))
		}
		if _, err = fmt.Fprintf(c.stdout, "Service: installed=%t loaded=%t active=%t enabled=%t stale=%t mode=%s\nLast run: %s; exit code: %s\n", report.Installed, report.Loaded, report.Active, report.Enabled, report.Stale, report.Mode, report.LastRun, serviceExit(report.ExitCode)); err != nil {
			return classified(err)
		}
		if report.BinaryMissing {
			if _, err = fmt.Fprintf(c.stdout, "Service executable %s no longer exists (removed by a package upgrade or uninstall?); run kitbag service install with the installed kitbag to point the service at it.\n", report.Binary); err != nil {
				return classified(err)
			}
		}
		return classified(c.printLastServiceSync(result))
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable service_schema=1 JSON")
	return cmd
}

func serviceExit(code *int) string {
	if code == nil {
		return "unknown"
	}
	return strconv.Itoa(*code)
}

func (c *commandLine) printLastServiceSync(report serviceReport) error {
	if report.LastSync != nil {
		last := report.LastSync
		_, err := fmt.Fprintf(c.stdout, "Last sync: %s; exit code: %d; %d changed; %s\n", c.clock.Describe(last.At), last.Result, last.Changed, last.Repository)
		return err
	}
	_, err := fmt.Fprintln(c.stdout, "Last sync: unknown. "+report.SyncError)
	return err
}
