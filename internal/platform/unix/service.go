//go:build darwin || linux

package unix

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ServiceRunner executes supervisor commands with an explicit environment.
// Nonzero process statuses are returned separately from startup/cancel errors.
type ServiceRunner func(context.Context, string, []string, []string) (string, int, error)

// WithServiceRunner injects supervisor execution without contacting a real manager.
func WithServiceRunner(run ServiceRunner) Option {
	return func(h *Host) { h.serviceRun = run }
}

func execService(ctx context.Context, name string, args, env []string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: fixed supervisor name, separate arguments, no shell.
	cmd.Env = env
	cmd.WaitDelay = time.Second
	output, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(output), exit.ExitCode(), nil
	}
	return string(output), 0, err
}

func (h *Host) serviceCommand(ctx context.Context, name string, args ...string) (string, int, error) {
	bounded, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	env := []string{"LC_ALL=C"}
	for _, key := range []string{"HOME", "PATH", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_CONFIG_HOME"} {
		if value, ok := h.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return h.serviceRun(bounded, name, args, env)
}

func (h *Host) serviceChecked(ctx context.Context, args ...string) error {
	_, code, err := h.serviceCommand(ctx, serviceProgram, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s %s failed (exit %d)", serviceProgram, strings.Join(args, " "), code)
	}
	return nil
}

const serviceTimeout = 10 * time.Second
