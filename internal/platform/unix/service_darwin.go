package unix

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// ServiceLayout resolves the per-user LaunchAgents directory and executable.
// A Homebrew Cellar executable is replaced by its stable opt or bin link.
func (h *Host) ServiceLayout() (platform.ServiceLayout, error) {
	home, err := h.HomeDir()
	if err != nil {
		return platform.ServiceLayout{}, err
	}
	binary, err := h.executable()
	if err == nil {
		binary = stableExecutable(binary)
	}
	user, _ := h.LookupEnv("USER")
	return platform.ServiceLayout{Manager: "launchd", Directory: filepath.Join(home, "Library", "LaunchAgents"), Binary: binary, User: user}, err
}

// ServiceControl never targets another user's launchd domain.
func (h *Host) ServiceControl(ctx context.Context, operation string, _ bool) error {
	target := serviceDomain() + "/" + platform.ServiceLabel
	switch operation {
	case "reload":
		return nil
	case "enable", "disable":
		return h.serviceChecked(ctx, operation, target)
	case "stop":
		return h.serviceChecked(ctx, "bootout", target)
	case "start":
		status, err := h.ServiceInspect(ctx, false)
		if err != nil {
			return err
		}
		if status.Loaded {
			return h.serviceChecked(ctx, "kickstart", target)
		}
		layout, err := h.ServiceLayout()
		if err != nil {
			return err
		}
		return h.serviceChecked(ctx, "bootstrap", serviceDomain(), filepath.Join(layout.Directory, platform.ServiceLabel+".plist"))
	default:
		return fmt.Errorf("service operation: %w", fs.ErrInvalid)
	}
}

// ServiceInspect parses launchctl print; exit 113 means the job is not loaded.
func (h *Host) ServiceInspect(ctx context.Context, timer bool) (platform.ServiceStatus, error) {
	data, code, err := h.serviceCommand(ctx, "launchctl", "print", serviceDomain()+"/"+platform.ServiceLabel)
	if err != nil {
		return platform.ServiceStatus{}, err
	}
	const missingService = 113
	if code != 0 && code != missingService {
		return platform.ServiceStatus{}, fmt.Errorf("launchctl print failed (exit %d)", code)
	}
	enabled, err := h.launchdEnabled(ctx)
	if err != nil {
		return platform.ServiceStatus{}, err
	}
	status := platform.ServiceStatus{Enabled: enabled}
	if code == missingService {
		return status, nil
	}
	status.Loaded = true
	status.Active = timer || strings.Contains(data, "state = running")
	// launchd does not expose a reliable wall-clock last run timestamp; the
	// persisted sync record provides it in the core report.
	status.ExitCode, err = launchdExit(data)
	return status, err
}

// launchdExit parses the "last exit code" value of launchctl print: a number,
// optionally followed by ": <description>", or a parenthesized note such as
// "(never exited)" for a job that has not exited since it was loaded, which is
// no observation (nil).
func launchdExit(data string) (*int, error) {
	var exit *int
	for line := range strings.SplitSeq(data, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "last exit code = ")
		if !ok || strings.HasPrefix(value, "(") {
			continue
		}
		number, _, _ := strings.Cut(value, ":")
		code, err := strconv.Atoi(strings.TrimSpace(number))
		if err != nil {
			return nil, fmt.Errorf("launchctl print: unexpected last exit code %q: %w", value, err)
		}
		exit = &code
	}
	return exit, nil
}

// ServiceLinger is always true: launchd has no systemd linger setting.
func (*Host) ServiceLinger(context.Context) (bool, error) { return true, nil }

func serviceDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func (h *Host) launchdEnabled(ctx context.Context) (bool, error) {
	data, code, err := h.serviceCommand(ctx, "launchctl", "print-disabled", serviceDomain())
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("launchctl print-disabled failed (exit %d)", code)
	}
	for line := range strings.SplitSeq(data, "\n") {
		label, value, ok := strings.Cut(strings.TrimSpace(line), " => ")
		if ok && label == `"`+platform.ServiceLabel+`"` {
			return strings.TrimSpace(value) != "true", nil
		}
	}
	return true, nil
}

const serviceProgram = "launchctl"
