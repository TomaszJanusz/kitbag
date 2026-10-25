package platform

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"runtime"
)

// Classification sentinels returned (wrapped) by every implementation.
var (
	// ErrUnsupported reports an operation the host cannot perform. It is the
	// standard library's sentinel, so filesystem errors such as ENOTSUP match it
	// too. Stubs wrap it with the name of the operation.
	ErrUnsupported = errors.ErrUnsupported
	// ErrCrossDevice reports a rename between two filesystems.
	ErrCrossDevice = errors.New("cross-device rename")
	// ErrIsDir reports a directory where a file or symbolic link was expected.
	ErrIsDir = errors.New("is a directory")
	// ErrNotDir reports a non-directory where a directory was expected.
	ErrNotDir = errors.New("not a directory")
	// ErrNotEmpty reports removal of a directory that still has entries.
	ErrNotEmpty = errors.New("directory not empty")
	// ErrNotLink reports a symbolic-link operation on another kind of entry.
	ErrNotLink = errors.New("not a symbolic link")
	// ErrLocked reports a lock that another holder already owns.
	ErrLocked = errors.New("locked by another holder")
	// ErrNoDaemon means no listener is running; clients may execute locally.
	ErrNoDaemon = errors.New("local daemon is not running")
)

// Host is the complete set of host operations. Only cmd/kitbag constructs one;
// other packages accept it, or the narrower [FS] or [Env].
type Host interface {
	FS
	Env
}

// FS is the host filesystem. See the package documentation for path rules,
// replacement semantics and error classification.
type FS interface {
	// Lstat describes path without following a final symbolic link.
	Lstat(path string) (fs.FileInfo, error)
	// Stat describes path, following symbolic links.
	Stat(path string) (fs.FileInfo, error)
	// ReadFile returns the contents of the file at path, following links.
	ReadFile(path string) ([]byte, error)
	// ReadDir lists the directory at path sorted by name, following links.
	ReadDir(path string) ([]fs.DirEntry, error)
	// ReadLink returns the target of the symbolic link at path verbatim.
	ReadLink(path string) (string, error)
	// IsManagedLink reports whether path is a symbolic link whose target lies
	// within one of roots. A relative target is resolved against the link's
	// directory; the comparison is lexical (no further links are followed),
	// whole-component and case-sensitive, and the target need not exist. It
	// returns false without error for other kinds of entries.
	IsManagedLink(path string, roots ...string) (bool, error)
	// MkdirAll creates path and any missing parents, each with exactly perm
	// (independent of the process umask). Existing directories, including
	// links to directories, are accepted unchanged.
	MkdirAll(path string, perm fs.FileMode) error
	// WriteAtomic replaces the file or symbolic link at path with a regular
	// file holding data and exactly perm. The parent directory must exist.
	WriteAtomic(path string, data []byte, perm fs.FileMode) error
	// Link replaces the file or symbolic link at path with a symbolic link to
	// target, which is stored verbatim and need not exist. The parent
	// directory must exist. The entry is never missing during replacement; on
	// macOS a concurrent read through the old link can fail transiently with
	// EINVAL (fs.ErrInvalid).
	Link(target, path string) error
	// Rename moves oldpath, of any kind, to newpath, which must not exist. A
	// final symbolic link is moved, not followed.
	Rename(oldpath, newpath string) error
	// Remove deletes a file, a symbolic link (never its target) or an empty
	// directory.
	Remove(path string) error
	// Chmod sets the permission bits of a file or directory to exactly perm.
	// It refuses symbolic links so that it never changes a link's target.
	Chmod(path string, perm fs.FileMode) error
	// TryLock takes an exclusive lock on the file at path, creating it with
	// mode 0600 when missing, without waiting: a lock already held, by this or
	// another process, fails with ErrLocked. The lock is released by Unlock or
	// when the process exits; the file itself is left in place.
	TryLock(path string) (Unlocker, error)
	// ListenLocal creates a 0600 socket in an owned, real 0700 directory.
	// Existing entries are never removed. Accept verifies the peer UID and
	// silently closes foreign connections. Close removes the owned socket.
	ListenLocal(path string) (net.Listener, error)
	// DialLocal verifies directory/socket permissions, ownership and peer UID.
	// Only a missing socket or connection refusal matches ErrNoDaemon.
	DialLocal(ctx context.Context, path string) (net.Conn, error)
}

// Unlocker releases a lock taken with [FS.TryLock]. A second Unlock fails with
// fs.ErrClosed.
type Unlocker interface {
	Unlock() error
}

// Env is the host environment: variables, directories, identity and desktop
// integration.
type Env interface {
	// LookupEnv returns the value of an environment variable and whether it
	// is set.
	LookupEnv(key string) (string, bool)
	// HomeDir returns the user's home directory.
	HomeDir() (string, error)
	// Dirs returns kitbag's own directories. They may not exist yet.
	Dirs() (Dirs, error)
	// RuntimeDir returns the socket directory: XDG_RUNTIME_DIR/kitbag, or
	// State/run when XDG_RUNTIME_DIR is absent or relative. It may not exist.
	RuntimeDir() (string, error)
	// AgentHome resolves an agent's home directory. When the environment
	// variable named override holds a value it must be an absolute path and
	// is returned cleaned; an empty or unset variable, or an empty override,
	// selects rel, a slash-separated path below the home directory.
	AgentHome(override, rel string) (string, error)
	// Hostname returns the host name reported by the system, unmodified.
	Hostname() (string, error)
	// Open opens an absolute file path or an http(s) URL with the desktop's
	// default handler. Other targets fail with fs.ErrInvalid.
	Open(target string) error
	// IsTerminal reports whether the file descriptor refers to a terminal.
	IsTerminal(fd uintptr) bool
}

// Dirs are kitbag's per-user directories, each specific to kitbag.
type Dirs struct {
	// Config holds config.toml, the machine's settings.
	Config string
	// Data holds the working copies of configuration repositories.
	Data string
	// State holds state.json, its history and lock.
	State string
	// Cache holds data that can be rebuilt, such as the skill catalog index.
	Cache string
	// LogDir holds bounded structured daemon logs below State/log.
	LogDir string
}

// UnsupportedOSError reports an operating system on which kitbag refuses to
// run. It matches [ErrUnsupported].
type UnsupportedOSError struct {
	// Message is shown to the user verbatim.
	Message string
}

// Error returns the user-facing message.
func (e *UnsupportedOSError) Error() string {
	return e.Message
}

// Unwrap returns [ErrUnsupported].
func (*UnsupportedOSError) Unwrap() error {
	return ErrUnsupported
}

// CheckOS reports whether kitbag runs on goos, a runtime.GOOS value. Windows
// compiles but is refused until the M6 compatibility layer replaces its stubs.
func CheckOS(goos string) error {
	if goos == "windows" {
		return &UnsupportedOSError{Message: "Windows is not supported before 1.x"}
	}
	return nil
}

// DefaultMode returns the platform materialization default. Windows runtime
// support remains deferred; cross-compilation uses its future copy default.
func DefaultMode() string {
	if runtime.GOOS == "windows" {
		return "copy"
	}
	return "link"
}
