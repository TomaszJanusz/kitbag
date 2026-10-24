//go:build windows

package windows

import (
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Host is the Windows platform stub. Create it with [New].
type Host struct{}

var _ platform.Host = (*Host)(nil)

// New returns the stub host.
func New() *Host {
	return &Host{}
}

// unsupported reports op as not implemented on Windows.
func unsupported(op string) error {
	return fmt.Errorf("windows: %s: %w", op, platform.ErrUnsupported)
}

// Lstat is not implemented on Windows yet.
func (*Host) Lstat(string) (fs.FileInfo, error) { return nil, unsupported("Lstat") }

// Stat is not implemented on Windows yet.
func (*Host) Stat(string) (fs.FileInfo, error) { return nil, unsupported("Stat") }

// ReadFile is not implemented on Windows yet.
func (*Host) ReadFile(string) ([]byte, error) { return nil, unsupported("ReadFile") }

// ReadDir is not implemented on Windows yet.
func (*Host) ReadDir(string) ([]fs.DirEntry, error) { return nil, unsupported("ReadDir") }

// ReadLink is not implemented on Windows yet.
func (*Host) ReadLink(string) (string, error) { return "", unsupported("ReadLink") }

// IsManagedLink is not implemented on Windows yet.
func (*Host) IsManagedLink(string, ...string) (bool, error) {
	return false, unsupported("IsManagedLink")
}

// MkdirAll is not implemented on Windows yet.
func (*Host) MkdirAll(string, fs.FileMode) error { return unsupported("MkdirAll") }

// WriteAtomic is not implemented on Windows yet.
func (*Host) WriteAtomic(string, []byte, fs.FileMode) error { return unsupported("WriteAtomic") }

// Link is not implemented on Windows yet.
func (*Host) Link(_, _ string) error { return unsupported("Link") }

// Rename is not implemented on Windows yet.
func (*Host) Rename(_, _ string) error { return unsupported("Rename") }

// Remove is not implemented on Windows yet.
func (*Host) Remove(string) error { return unsupported("Remove") }

// Chmod is not implemented on Windows yet.
func (*Host) Chmod(string, fs.FileMode) error { return unsupported("Chmod") }

// TryLock is not implemented on Windows yet.
func (*Host) TryLock(string) (platform.Unlocker, error) { return nil, unsupported("TryLock") }

// LookupEnv reports every variable as unset until M6.
func (*Host) LookupEnv(string) (string, bool) { return "", false }

// HomeDir is not implemented on Windows yet.
func (*Host) HomeDir() (string, error) { return "", unsupported("HomeDir") }

// Dirs is not implemented on Windows yet.
func (*Host) Dirs() (platform.Dirs, error) { return platform.Dirs{}, unsupported("Dirs") }

// AgentHome is not implemented on Windows yet.
func (*Host) AgentHome(_, _ string) (string, error) { return "", unsupported("AgentHome") }

// Hostname is not implemented on Windows yet.
func (*Host) Hostname() (string, error) { return "", unsupported("Hostname") }

// Open is not implemented on Windows yet.
func (*Host) Open(string) error { return unsupported("Open") }

// IsTerminal reports false until M6.
func (*Host) IsTerminal(uintptr) bool { return false }
