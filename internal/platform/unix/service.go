//go:build darwin || linux

package unix

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
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

// WithExecutable replaces discovery of the resolved running executable, so
// tests can model package-manager layouts such as a Homebrew Cellar.
func WithExecutable(executable func() (string, error)) Option {
	return func(h *Host) { h.executable = executable }
}

// stableExecutable maps a versioned Homebrew keg path,
// <prefix>/Cellar/<formula>/<version>/<rest>, to the version-independent
// <prefix>/opt/<formula>/<rest> link, or for bin/<name> and libexec/<name>
// to <prefix>/bin/<name>,
// but only when that link resolves to resolved now. A brew upgrade repoints
// these links while cleanup deletes the old keg. Other paths are unchanged.
func stableExecutable(resolved string) string {
	const kegDepth = 3 // formula, version and at least one entry inside the keg
	parts := strings.Split(resolved, string(filepath.Separator))
	for i := len(parts) - 1 - kegDepth; i >= 1; i-- {
		if parts[i] != "Cellar" {
			continue
		}
		prefix := filepath.Join(append([]string{string(filepath.Separator)}, parts[1:i]...)...)
		rest := filepath.Join(parts[i+kegDepth:]...)
		candidates := []string{filepath.Join(prefix, "opt", parts[i+1], rest)}
		if dir := filepath.Dir(rest); dir == "bin" || dir == "libexec" {
			candidates = append(candidates, filepath.Join(prefix, "bin", filepath.Base(rest)))
		}
		for _, candidate := range candidates {
			if target, err := filepath.EvalSymlinks(candidate); err == nil && target == resolved {
				return candidate
			}
		}
		return resolved
	}
	return resolved
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
