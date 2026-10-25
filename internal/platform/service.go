package platform

import "context"

// Service is the user service supervisor. No operation requests privileges.
// Control accepts reload, enable, start, stop and disable, and addresses only
// KitBag's fixed unit names. timer selects the timer rather than the daemon.
type Service interface {
	ServiceLayout() (ServiceLayout, error)
	ServiceControl(ctx context.Context, operation string, timer bool) error
	ServiceInspect(ctx context.Context, timer bool) (ServiceStatus, error)
	ServiceLinger(ctx context.Context) (bool, error)
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

// ServiceLabel is the launchd label and systemd unit basename.
const ServiceLabel = "dev.kitbag.agent"
