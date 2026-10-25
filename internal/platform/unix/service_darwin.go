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
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
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

// ServiceInspect parses launchctl print.
func (h *Host) ServiceInspect(ctx context.Context, timer bool) (platform.ServiceStatus, error) {
	data, loaded, err := h.launchdPrint(ctx, platform.ServiceLabel)
	if err != nil {
		return platform.ServiceStatus{}, err
	}
	enabled, err := h.launchdEnabled(ctx, platform.ServiceLabel)
	if err != nil {
		return platform.ServiceStatus{}, err
	}
	status := platform.ServiceStatus{Enabled: enabled}
	if !loaded {
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

// UnitInspect reads another job's state in this user's GUI domain without
// loading or starting it: loaded when launchctl print finds the label, enabled
// unless the domain's override database disables it.
func (h *Host) UnitInspect(ctx context.Context, label string) (platform.UnitStatus, error) {
	if err := check.Unit(label); err != nil {
		return platform.UnitStatus{}, err
	}
	_, loaded, err := h.launchdPrint(ctx, label)
	if err != nil {
		return platform.UnitStatus{}, err
	}
	enabled, err := h.launchdEnabled(ctx, label)
	return platform.UnitStatus{Loaded: loaded, Enabled: enabled}, err
}

// launchdPrint describes a job in this user's GUI domain; exit 113 means the
// job is not loaded.
func (h *Host) launchdPrint(ctx context.Context, label string) (string, bool, error) {
	data, code, err := h.serviceCommand(ctx, "launchctl", "print", serviceDomain()+"/"+label)
	if err != nil {
		return "", false, err
	}
	const missingService = 113
	if code != 0 && code != missingService {
		return "", false, fmt.Errorf("launchctl print failed (exit %d)", code)
	}
	return data, code == 0, nil
}

// ServiceLinger is always true: launchd has no systemd linger setting.
func (*Host) ServiceLinger(context.Context) (bool, error) { return true, nil }

func serviceDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// launchdEnabled reads the override database. Older macOS prints "true" for a
// disabled label, newer releases "disabled".
func (h *Host) launchdEnabled(ctx context.Context, label string) (bool, error) {
	data, code, err := h.serviceCommand(ctx, "launchctl", "print-disabled", serviceDomain())
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("launchctl print-disabled failed (exit %d)", code)
	}
	for line := range strings.SplitSeq(data, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), " => ")
		if ok && name == `"`+label+`"` {
			value = strings.TrimSpace(value)
			return value != "true" && value != "disabled", nil
		}
	}
	return true, nil
}

const serviceProgram = "launchctl"
