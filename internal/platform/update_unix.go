//go:build darwin || linux

package platform

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Executable returns the real executable path, resolving installer symlinks.
func Executable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

// ProbeExecutable confirms the replacement's version in a clean environment.
func ProbeExecutable(ctx context.Context, path string) (string, error) {
	const timeout = 5 * time.Second
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(bounded, path, "version", "--bare")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	data, err := cmd.Output()
	return strings.TrimSpace(string(data)), err
}

// Reexec replaces the process with the verified binary and original environment.
func Reexec(ctx context.Context, path string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fs.ErrInvalid
	}
	return syscall.Exec(path, append([]string{path}, args...), os.Environ()) //nolint:gosec // G204: core verified and confirmed the replacement; no shell.
}
