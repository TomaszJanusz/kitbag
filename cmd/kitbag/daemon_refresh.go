package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// startupRefresh runs before the listener and scheduler, so a requested restart
// cannot interrupt sync. Failure is visible and leaves the daemon usable.
func (c *commandLine) startupRefresh(ctx context.Context) (bool, error) {
	restart, err := c.userService().AutoRefresh(ctx, toolVersion(), time.Now(), func() error {
		layout, layoutErr := c.host.ServiceLayout()
		if layoutErr != nil {
			return layoutErr
		}
		dirs, dirErr := c.host.Dirs()
		if dirErr != nil {
			return dirErr
		}
		return platform.DetachServiceRefresh(layout.Binary, c.configPath, dirs.LogDir)
	})
	if err != nil {
		_, writeErr := fmt.Fprintf(c.stderr, "Service refresh deferred: %v; run kitbag service refresh.\n", err)
		return false, writeErr
	}
	if restart {
		c.code = apply.Code(service.RestartExitCode)
	}
	return restart, nil
}
