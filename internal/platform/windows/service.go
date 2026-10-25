//go:build windows

package windows

import (
	"context"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// ServiceLayout is not implemented on Windows yet.
func (*Host) ServiceLayout() (platform.ServiceLayout, error) {
	return platform.ServiceLayout{}, unsupported("ServiceLayout")
}

// ServiceControl is not implemented on Windows yet.
func (*Host) ServiceControl(context.Context, string, bool) error {
	return unsupported("ServiceControl")
}

// ServiceInspect is not implemented on Windows yet.
func (*Host) ServiceInspect(context.Context, bool) (platform.ServiceStatus, error) {
	return platform.ServiceStatus{}, unsupported("ServiceInspect")
}

// ServiceLinger is not implemented on Windows yet.
func (*Host) ServiceLinger(context.Context) (bool, error) { return false, unsupported("ServiceLinger") }
