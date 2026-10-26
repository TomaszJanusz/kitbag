package platform

import "context"

// Service is the user service supervisor. No operation requests privileges.
// Control accepts reload, enable, start, restart (systemd), stop and disable, and addresses only
// KitBag's fixed unit names. timer selects the timer rather than the daemon.
type Service interface {
	ServiceLayout() (ServiceLayout, error)
	ServiceControl(ctx context.Context, operation string, timer bool) error
	ServiceInspect(ctx context.Context, timer bool) (ServiceStatus, error)
	ServiceLinger(ctx context.Context) (bool, error)
	// UnitInspect reads the state of another user unit by name, a systemd unit
	// file name or a launchd label, without loading, starting or changing it.
	// A name that is not one plain path element fails with fs.ErrInvalid.
	UnitInspect(ctx context.Context, name string) (UnitStatus, error)
}

// ServiceLayout describes host-specific paths and the resolved running binary.
type ServiceLayout struct {
	Manager, Directory, Binary, User string
}

// ServiceStatus is the supervisor's last known state; empty last-run/exit fields
// mean the supervisor has no observation yet (not a successful run).
type ServiceStatus struct {
	ExitCode *int   `json:"exit_code"`
	LastRun  string `json:"last_run"`
	Loaded   bool   `json:"loaded"`
	Active   bool   `json:"active"`
	Enabled  bool   `json:"enabled"`
}

// UnitStatus observes a user unit KitBag does not own, such as a legacy timer.
// Loaded means the manager holds the job now (systemd: active or activating;
// launchd: bootstrapped). Enabled means it is loaded again at the next login
// (systemd: an enabled unit file; launchd: not disabled). An unknown unit is
// neither.
type UnitStatus struct {
	Loaded  bool `json:"loaded"`
	Enabled bool `json:"enabled"`
}

// ServiceLabel is the launchd label and systemd unit basename.
const ServiceLabel = "dev.kitbag.agent"
