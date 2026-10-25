package unix

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// ServiceLayout resolves the systemd user unit directory and executable.
func (h *Host) ServiceLayout() (platform.ServiceLayout, error) {
	dirs, err := h.Dirs()
	if err != nil {
		return platform.ServiceLayout{}, err
	}
	binary, err := platform.Executable()
	user, _ := h.LookupEnv("USER")
	return platform.ServiceLayout{Manager: "systemd", Directory: filepath.Join(filepath.Dir(dirs.Config), "systemd", "user"), Binary: binary, User: user}, err
}

func serviceUnit(timer bool) string {
	if timer {
		return platform.ServiceLabel + ".timer"
	}
	return platform.ServiceLabel + ".service"
}

// ServiceControl addresses only KitBag's user units, never system services.
func (h *Host) ServiceControl(ctx context.Context, operation string, timer bool) error {
	if operation == "reload" {
		return h.serviceChecked(ctx, "--user", "daemon-reload")
	}
	switch operation {
	case "enable", "start", "stop", "disable":
		return h.serviceChecked(ctx, "--user", operation, serviceUnit(timer))
	default:
		return fmt.Errorf("service operation: %w", fs.ErrInvalid)
	}
}

// ServiceInspect reads state without starting or loading a unit.
func (h *Host) ServiceInspect(ctx context.Context, timer bool) (platform.ServiceStatus, error) {
	data, code, err := h.serviceCommand(ctx, "systemctl", "--user", "show", serviceUnit(timer), "--property=LoadState,ActiveState,UnitFileState,ExecMainStartTimestamp,ExecMainStatus,Result")
	if err != nil {
		return platform.ServiceStatus{}, err
	}
	if property(data, "LoadState") == "not-found" {
		return platform.ServiceStatus{}, nil
	}
	if code != 0 {
		return platform.ServiceStatus{}, fmt.Errorf("systemctl show failed (exit %d)", code)
	}
	status := platform.ServiceStatus{Loaded: property(data, "LoadState") == "loaded", Active: property(data, "ActiveState") == "active", Enabled: property(data, "UnitFileState") == "enabled", LastRun: property(data, "ExecMainStartTimestamp")}
	if value := property(data, "ExecMainStatus"); value != "" && status.LastRun != "" {
		exit, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			return status, parseErr
		}
		status.ExitCode = &exit
	}
	if timer {
		return h.timerStatus(ctx, status)
	}
	return status, nil
}

func (h *Host) timerStatus(ctx context.Context, status platform.ServiceStatus) (platform.ServiceStatus, error) {
	run, err := h.ServiceInspect(ctx, false)
	status.LastRun, status.ExitCode = run.LastRun, run.ExitCode
	return status, err
}

// ServiceLinger reports whether this user's manager survives logout.
func (h *Host) ServiceLinger(ctx context.Context) (bool, error) {
	user, _ := h.LookupEnv("USER")
	data, code, err := h.serviceCommand(ctx, "loginctl", "show-user", user, "-p", "Linger")
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("loginctl show-user failed (exit %d)", code)
	}
	return strings.TrimSpace(data) == "Linger=yes", nil
}

const serviceProgram = "systemctl"

func property(data, name string) string {
	for line := range strings.SplitSeq(data, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), name+"="); ok {
			return value
		}
	}
	return ""
}
