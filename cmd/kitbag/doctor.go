package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/doctor"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

const reportPerm fs.FileMode = 0o600

func (c *commandLine) doctorCommand() *cobra.Command {
	var asJSON, strict bool
	var reportPath string
	cmd := &cobra.Command{Use: "doctor", Short: "Explain synchronization problems and suggest fixes", Long: "Check local sync prerequisites and remote reachability without applying changes. --report writes redacted local JSON and skips network probes. WARN exits 0 unless --strict; FAIL exits 1, or 4 when min_tool_version is unmet.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, err := c.doctorService(reportPath != "")
		if err != nil {
			return classified(err)
		}
		result := service.Run(cmd.Context())
		c.code = result.Code(strict)
		if reportPath != "" {
			if err = c.writeDoctorReport(reportPath, result); err != nil {
				return classified(err)
			}
		}
		return classified(c.printDoctor(result, asJSON))
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable doctor_schema=1 JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit 1 for warnings as well as failures")
	cmd.Flags().StringVar(&reportPath, "report", "", "Write a local redacted JSON report (skips network probes)")
	return cmd
}

func (c *commandLine) doctorService(offline bool) (doctor.Service, error) {
	repo, err := c.config.WorkingCopy(c.host)
	if err != nil {
		return doctor.Service{}, err
	}
	store, err := state.New(c.host, c.host, state.Options{})
	if err != nil {
		return doctor.Service{}, err
	}
	return doctor.Service{
		Host: c.host, Store: store, Repository: repo, Config: c.config, Version: toolVersion(), HostFlag: c.flags.host, ModeFlag: c.flags.mode, Offline: offline,
		Open: func(ctx context.Context) (vcs.Backend, vcs.Resolution, error) {
			return vcs.Open(ctx, c.selection, gitFactories(c.host))
		},
		VersionProbe: func(ctx context.Context, name string) (string, error) {
			return system.ProbeVersion(ctx, c.host, system.ExecRunner{}, name)
		},
	}, nil
}

func (c *commandLine) writeDoctorReport(path string, result doctor.Report) error {
	home, err := c.host.HomeDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(result.Redact(home), "", "  ")
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return c.host.WriteAtomic(absolute, append(data, '\n'), reportPerm)
}

func (c *commandLine) printDoctor(result doctor.Report, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	for _, status := range []doctor.Status{doctor.OK, doctor.Warn, doctor.Fail} {
		if _, err := fmt.Fprintln(c.stdout, string(status)+":"); err != nil {
			return err
		}
		for _, check := range result.Checks {
			if check.Status != status {
				continue
			}
			line := fmt.Sprintf("  %s: %s", check.ID, check.Message)
			if check.Fix != "" {
				line += "; fix: " + check.Fix
			}
			if _, err := fmt.Fprintln(c.stdout, line); err != nil {
				return err
			}
		}
	}
	return nil
}
