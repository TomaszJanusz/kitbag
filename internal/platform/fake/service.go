package fake

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

// WithServiceLayout replaces the default systemd layout, for example with a
// launchd manager or a package-manager executable path. Like the default, the
// executable is not created in the filesystem.
func WithServiceLayout(layout platform.ServiceLayout) Option {
	return func(h *Host) { h.serviceLayout = &layout }
}

// ServiceLayout supplies deterministic systemd paths without host discovery.
func (h *Host) ServiceLayout() (platform.ServiceLayout, error) {
	if h.serviceLayout != nil {
		return *h.serviceLayout, nil
	}
	dirs, err := h.Dirs()
	return platform.ServiceLayout{Manager: "systemd", Directory: filepath.Join(filepath.Dir(dirs.Config), "systemd", "user"), Binary: filepath.FromSlash("/usr/bin/kitbag"), User: "fake"}, err
}

// ServiceControl models supervisor state and records calls for assertions.
func (h *Host) ServiceControl(ctx context.Context, operation string, timer bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serviceCalls = append(h.serviceCalls, fmt.Sprintf("%s timer=%t", operation, timer))
	if h.serviceState == nil {
		h.serviceState = map[bool]platform.ServiceStatus{}
	}
	status := h.serviceState[timer]
	switch operation {
	case "reload":
	case "enable":
		status.Enabled = true
		status.Loaded = true
	case "disable":
		status.Enabled = false
	case "start", "restart":
		status.Active = true
		status.Loaded = true
	case "stop":
		status.Active = false
		status.Loaded = false
	default:
		return fs.ErrInvalid
	}
	h.serviceState[timer] = status
	return nil
}

// ServiceInspect returns the simulated supervisor state.
func (h *Host) ServiceInspect(ctx context.Context, timer bool) (platform.ServiceStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.serviceState[timer], ctx.Err()
}

// SetUnit sets the state UnitInspect reports for another user unit; any other
// valid name is an unknown unit.
func (h *Host) SetUnit(name string, status platform.UnitStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.units[name] = status
}

// UnitInspect reports the state set with [Host.SetUnit].
func (h *Host) UnitInspect(ctx context.Context, name string) (platform.UnitStatus, error) {
	if err := check.Unit(name); err != nil {
		return platform.UnitStatus{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.units[name], ctx.Err()
}

// ServiceLinger reports enabled linger without executing loginctl.
func (*Host) ServiceLinger(ctx context.Context) (bool, error) { return true, ctx.Err() }

// ServiceCalls returns a copy of recorded supervisor actions.
func (h *Host) ServiceCalls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.serviceCalls...)
}
