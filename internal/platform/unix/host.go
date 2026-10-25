//go:build darwin || linux

package unix

import (
	"os"
	"os/exec"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/xdg"
)

// Host is the macOS and Linux platform. Create it with [New].
type Host struct {
	lookup xdg.Lookup
	run    func(name string, args ...string) error
	// The durability hooks are replaced only by white-box tests.
	syncFile func(*os.File) error
	syncDir  func(dir string) error
	rename   func(oldpath, newpath string) error
}

var _ platform.Host = (*Host)(nil)

// Option configures a [Host].
type Option func(*Host)

// WithEnv replaces the process environment with lookup, so tests never read
// the real HOME or XDG variables.
func WithEnv(lookup func(key string) (string, bool)) Option {
	return func(h *Host) {
		h.lookup = lookup
	}
}

// WithCommandRunner replaces how [Host.Open] runs the desktop opener, so tests
// never launch a browser.
func WithCommandRunner(run func(name string, args ...string) error) Option {
	return func(h *Host) {
		h.run = run
	}
}

// New returns a host backed by the process environment and filesystem.
func New(opts ...Option) *Host {
	h := &Host{
		lookup:   os.LookupEnv,
		run:      runCommand,
		syncFile: (*os.File).Sync,
		syncDir:  syncDir,
		rename:   os.Rename,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// LookupEnv returns the value of an environment variable.
func (h *Host) LookupEnv(key string) (string, bool) {
	return h.lookup(key)
}

// HomeDir returns $HOME, which must be absolute.
func (h *Host) HomeDir() (string, error) {
	return xdg.Home(h.lookup)
}

// Dirs returns kitbag's XDG directories.
func (h *Host) Dirs() (platform.Dirs, error) {
	return xdg.Dirs(h.lookup)
}

// RuntimeDir returns the private local socket directory.
func (h *Host) RuntimeDir() (string, error) {
	return xdg.RuntimeDir(h.LookupEnv)
}

// AgentHome resolves an agent's home directory.
func (h *Host) AgentHome(override, rel string) (string, error) {
	return xdg.AgentHome(h.lookup, override, rel)
}

// Hostname returns the kernel's host name.
func (*Host) Hostname() (string, error) {
	return os.Hostname()
}

// Open passes a validated target to open(1) on macOS or xdg-open(1) on Linux
// and waits for the opener, which returns once it has handed the target over.
func (h *Host) Open(target string) error {
	if err := check.OpenTarget(target); err != nil {
		return err
	}
	return h.run(opener, target)
}

// runCommand runs a desktop opener without a shell. Open validates the target
// as an absolute path or an http(s) URL and opener is a constant.
func runCommand(name string, args ...string) error {
	return exec.Command(name, args...).Run() //nolint:gosec // G204: constant opener, target validated by check.OpenTarget, no shell.
}
