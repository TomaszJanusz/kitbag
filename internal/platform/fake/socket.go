package fake

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"path/filepath"
	"sync"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

const (
	socketPerm  fs.FileMode = 0o600
	runtimePerm fs.FileMode = 0o700
)

// ListenLocal models a private socket with in-memory duplex pipes.
func (h *Host) ListenLocal(path string) (net.Listener, error) {
	if err := check.Path("listen", path); err != nil {
		return nil, err
	}
	if err := h.MkdirAll(filepath.Dir(path), runtimePerm); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.privateEntry(filepath.Dir(path), fs.ModeDir, runtimePerm); err != nil {
		return nil, err
	}
	w, err := h.walk(path, false)
	if err != nil {
		return nil, err
	}
	if w.node != nil {
		return nil, fs.ErrExist
	}
	w.parent.children[w.name] = &node{mode: fs.ModeSocket | socketPerm}
	listener := &localListener{host: h, path: path, connections: make(chan net.Conn), done: make(chan struct{})}
	h.listeners[path] = listener
	return listener, nil
}

// DialLocal connects as the owner; missing or closed listeners match ErrNoDaemon.
func (h *Host) DialLocal(ctx context.Context, path string) (net.Conn, error) {
	return h.DialLocalAs(ctx, path, true)
}

// DialLocalAs simulates the kernel peer-credential decision. A foreign user is
// refused even if a test deliberately changes the permission bits.
func (h *Host) DialLocalAs(ctx context.Context, path string, owner bool) (net.Conn, error) {
	if err := check.Path("dial", path); err != nil {
		return nil, err
	}
	if !owner {
		return nil, fs.ErrPermission
	}
	listener, err := h.endpoint(path)
	if err != nil {
		return nil, err
	}
	client, server := net.Pipe()
	select {
	case listener.connections <- server:
		return client, nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-listener.done:
		err = platform.ErrNoDaemon
	}
	return nil, errors.Join(err, client.Close(), server.Close())
}

func (h *Host) endpoint(path string) (*localListener, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, entry := range []struct {
		path       string
		kind, perm fs.FileMode
	}{
		{path, fs.ModeSocket, socketPerm}, {filepath.Dir(path), fs.ModeDir, runtimePerm},
	} {
		if err := h.privateEntry(entry.path, entry.kind, entry.perm); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				err = errors.Join(platform.ErrNoDaemon, err)
			}
			return nil, err
		}
	}
	listener := h.listeners[path]
	if listener == nil {
		return nil, platform.ErrNoDaemon
	}
	return listener, nil
}

func (h *Host) privateEntry(path string, kind, perm fs.FileMode) error {
	n, err := h.existing("socket", path, false)
	if err != nil {
		return err
	}
	if n.mode.Type() != kind || n.mode.Perm() != perm {
		return fs.ErrPermission
	}
	return nil
}

type localListener struct {
	host        *Host
	connections chan net.Conn
	done        chan struct{}
	path        string
	once        sync.Once
}

// Accept waits for an owner connection or Close.
func (l *localListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

// Close unblocks Accept and removes the simulated socket.
func (l *localListener) Close() error {
	l.once.Do(func() {
		close(l.done)
		l.host.mu.Lock()
		defer l.host.mu.Unlock()
		delete(l.host.listeners, l.path)
		if w, err := l.host.walk(l.path, false); err == nil && w.parent != nil {
			delete(w.parent.children, w.name)
		}
	})
	return nil
}

// Addr returns the simulated Unix endpoint.
func (l *localListener) Addr() net.Addr { return localAddr(l.path) }

type localAddr string

// Network returns the local transport name.
func (localAddr) Network() string { return "unix" }

// String returns the socket path.
func (a localAddr) String() string { return string(a) }
