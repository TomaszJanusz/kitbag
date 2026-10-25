//go:build darwin || linux

package unix

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

const (
	socketPerm  fs.FileMode = 0o600
	runtimePerm fs.FileMode = 0o700
)

// ListenLocal creates a private socket and verifies credentials on every accept.
func (h *Host) ListenLocal(path string) (net.Listener, error) {
	if err := check.Path("listen", path); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := h.MkdirAll(dir, runtimePerm); err != nil {
		return nil, err
	}
	if err := privateEntry(dir, fs.ModeDir, runtimePerm); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			err = fs.ErrExist
		}
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, socketPerm); err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	return &localListener{UnixListener: listener, verify: verifyPeer}, nil
}

// DialLocal refuses unsafe endpoints and authenticates the daemon's UID.
func (*Host) DialLocal(ctx context.Context, path string) (net.Conn, error) {
	if err := check.Path("dial", path); err != nil {
		return nil, err
	}
	if err := privateEntry(path, fs.ModeSocket, socketPerm); err != nil {
		return nil, noDaemon(err)
	}
	if err := privateEntry(filepath.Dir(path), fs.ModeDir, runtimePerm); err != nil {
		return nil, noDaemon(err)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, noDaemon(err)
	}
	if err = verifyPeer(conn); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

func noDaemon(err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
		return errors.Join(platform.ErrNoDaemon, err)
	}
	return err
}

func privateEntry(path string, kind, perm fs.FileMode) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || fi.Mode().Type() != kind || fi.Mode().Perm() != perm || int64(stat.Uid) != int64(os.Geteuid()) {
		return fs.ErrPermission
	}
	return nil
}

type localListener struct {
	*net.UnixListener
	verify func(net.Conn) error
}

// Accept silently refuses foreign peers so one failed authorization cannot
// stop the daemon's accept loop.
func (l *localListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if l.verify(conn) == nil {
			return conn, nil
		}
		if err := conn.Close(); err != nil {
			return nil, err
		}
	}
}

func verifyPeer(conn net.Conn) error {
	local, ok := conn.(*net.UnixConn)
	if !ok {
		return fs.ErrPermission
	}
	raw, err := local.SyscallConn()
	if err != nil {
		return err
	}
	var peerErr error
	err = raw.Control(func(fd uintptr) { peerErr = peerOwner(fd) })
	return errors.Join(err, peerErr)
}
