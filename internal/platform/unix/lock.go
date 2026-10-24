//go:build darwin || linux

package unix

import (
	"errors"
	"io/fs"
	"os"
	"sync"
	"syscall"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

// lockPerm is the mode of a lock file created by TryLock.
const lockPerm fs.FileMode = 0o600

var errSymlinkLock = errors.New("refusing to lock through a symbolic link")

// fileLock is an flock(2) lock on an open file description.
type fileLock struct {
	f  *os.File
	mu sync.Mutex
}

// TryLock takes an exclusive flock(2) lock without waiting.
func (*Host) TryLock(path string) (platform.Unlocker, error) {
	if err := check.Path("lock", path); err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(path); err == nil {
		switch {
		case fi.IsDir():
			return nil, wrap("lock", path, platform.ErrIsDir)
		case fi.Mode()&fs.ModeSymlink != 0:
			return nil, check.Invalid("lock", path, errSymlinkLock)
		}
	}
	// flock(2) does not need write access; O_NOFOLLOW closes the race with a
	// symbolic link created after the check above.
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE|syscall.O_NOFOLLOW, lockPerm) //nolint:gosec // G304: the lock path is a validated absolute path chosen by kitbag.
	if err != nil {
		return nil, classify(err)
	}
	if err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = &classified{kind: platform.ErrLocked, err: err}
		}
		return nil, wrap("lock", path, errors.Join(err, f.Close()))
	}
	return &fileLock{f: f}, nil
}

// Unlock releases the lock and closes its file.
func (l *fileLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return fs.ErrClosed
	}
	f := l.f
	l.f = nil
	return errors.Join(flock(f, syscall.LOCK_UN), f.Close())
}

// flock applies how to the file's descriptor.
func flock(f *os.File, how int) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var lockErr error
	if err := conn.Control(func(fd uintptr) {
		lockErr = syscall.Flock(int(fd), how)
	}); err != nil {
		return err
	}
	return lockErr
}
