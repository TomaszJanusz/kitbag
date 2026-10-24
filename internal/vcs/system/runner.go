package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// waitDelay bounds how long a cancelled Git process may keep its output pipes
// open (for example through a credential helper it started).
const waitDelay = 5 * time.Second

// Command is one process to run. Env is the complete environment: nothing is
// inherited from kitbag's own process.
type Command struct {
	// Path is the absolute path of the executable.
	Path string
	Args []string
	// Dir is the working directory; it must be absolute.
	Dir string
	Env []string
}

// Result is the outcome of a process that ran to completion.
type Result struct {
	Stdout, Stderr []byte
	// ExitCode is the process's exit status.
	ExitCode int
}

// Runner starts processes. [ExecRunner] is the real implementation; tests
// inject fakes so parsing and environment handling run without Git.
type Runner interface {
	// LookPath returns the absolute path of the executable name found in
	// pathList (a PATH value), ignoring empty and relative entries.
	LookPath(name, pathList string) (string, error)
	// Run runs cmd to completion. A non-zero exit status is reported in
	// Result.ExitCode, not as an error; errors mean the process could not be
	// started or was cancelled.
	Run(ctx context.Context, cmd Command) (Result, error)
}

// ExecRunner runs processes with os/exec. It is the only code in kitbag
// outside internal/platform that starts processes.
type ExecRunner struct{}

var _ Runner = ExecRunner{}

// LookPath searches pathList itself rather than the process's PATH, so the
// caller's environment decides which Git runs.
func (ExecRunner) LookPath(name, pathList string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		// Relative entries (including "" for the current directory) would
		// make the result depend on the working directory.
		if !filepath.IsAbs(dir) {
			continue
		}
		path, err := exec.LookPath(filepath.Join(dir, name))
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, exec.ErrNotFound)
}

// Run starts cmd with exactly cmd.Env, no standard input and captured output.
func (ExecRunner) Run(ctx context.Context, cmd Command) (Result, error) {
	c := exec.CommandContext(ctx, cmd.Path, cmd.Args...) //nolint:gosec // G204: Path is the resolved Git binary; arguments are passed without a shell.
	c.Dir = cmd.Dir
	// A nil Env would inherit kitbag's environment.
	c.Env = append([]string{}, cmd.Env...)
	c.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr

	err := c.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return res, nil
	case ctx.Err() != nil:
		return res, fmt.Errorf("%s: %w", cmd.Path, ctx.Err())
	case errors.As(err, &exitErr) && exitErr.Exited():
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("run %s: %w", cmd.Path, err)
}
