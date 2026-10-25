package platform_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s suite) runSockets(t *testing.T) {
	t.Helper()
	t.Run("Socket/Private", s.socketPrivate)
	t.Run("Socket/RejectUnsafe", s.socketUnsafe)
	t.Run("Socket/Missing", s.socketMissing)
	t.Run("Env/RuntimeDir", s.envRuntime)
}

func (s suite) socketPrivate(t *testing.T) {
	t.Parallel()
	sub := s.socketSubject(t)
	path := filepath.Join(sub.root, "run", "api.sock")
	listener, err := sub.host.ListenLocal(path)
	mustNil(t, "listen", err)
	t.Cleanup(func() { mustNil(t, "close listener", listener.Close()) })
	wantKind(t, sub.host, filepath.Dir(path), fs.ModeDir, 0o700)
	wantKind(t, sub.host, path, fs.ModeSocket, 0o600)
	_, err = sub.host.ListenLocal(path)
	wantErr(t, "second listener", err, fs.ErrExist)
	done := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			done <- acceptErr
			return
		}
		_, copyErr := io.CopyN(conn, conn, 1)
		done <- errors.Join(copyErr, conn.Close())
	}()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	conn, err := sub.host.DialLocal(ctx, path)
	mustNil(t, "dial", err)
	roundTripSocket(t, conn)
	mustNil(t, "echo", <-done)
}

func roundTripSocket(t *testing.T, conn net.Conn) {
	t.Helper()
	defer func() { mustNil(t, "close client", conn.Close()) }()
	mustNil(t, "deadline", conn.SetDeadline(time.Now().Add(time.Second)))
	_, err := conn.Write([]byte("x"))
	mustNil(t, "write", err)
	got := make([]byte, 1)
	_, err = io.ReadFull(conn, got)
	mustNil(t, "read", err)
	if string(got) != "x" {
		t.Fatalf("socket read=%q", got)
	}
}

func (s suite) socketUnsafe(t *testing.T) {
	t.Parallel()
	sub := s.socketSubject(t)
	path := filepath.Join(sub.root, "run", "api.sock")
	mustNil(t, "create insecure directory", sub.host.MkdirAll(filepath.Dir(path), 0o755))
	_, err := sub.host.ListenLocal(path)
	wantErr(t, "insecure runtime", err, fs.ErrPermission)
	mustNil(t, "secure runtime", sub.host.Chmod(filepath.Dir(path), 0o700))
	listener, err := sub.host.ListenLocal(path)
	mustNil(t, "listen", err)
	t.Cleanup(func() { mustNil(t, "close", listener.Close()) })
	mustNil(t, "insecure socket", sub.host.Chmod(path, 0o666))
	_, err = sub.host.DialLocal(t.Context(), path)
	wantErr(t, "dial insecure socket", err, fs.ErrPermission)
	mustNil(t, "secure socket", sub.host.Chmod(path, 0o600))
	mustNil(t, "insecure directory", sub.host.Chmod(filepath.Dir(path), 0o755))
	_, err = sub.host.DialLocal(t.Context(), path)
	wantErr(t, "dial insecure directory", err, fs.ErrPermission)
}

func (s suite) envRuntime(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "relative", filepath.FromSlash("/runtime")} {
		host := s.newSubject(t, map[string]string{"HOME": home(), "XDG_RUNTIME_DIR": value}).host
		want := home(".local", "state", "kitbag", "run")
		if filepath.IsAbs(value) {
			want = filepath.Join(value, "kitbag")
		}
		got, err := host.RuntimeDir()
		mustNil(t, "runtime dir", err)
		if got != want {
			t.Errorf("runtime %q=%q, want %q", value, got, want)
		}
	}
}

func (s suite) socketMissing(t *testing.T) {
	t.Parallel()
	sub := s.socketSubject(t)
	path := filepath.Join(sub.root, "run", "api.sock")
	_, err := sub.host.DialLocal(t.Context(), path)
	wantErr(t, "absent runtime", err, platform.ErrNoDaemon)
	mustNil(t, "unused directory", sub.host.MkdirAll(filepath.Dir(path), 0o755))
	_, err = sub.host.DialLocal(t.Context(), path)
	wantErr(t, "absent socket", err, platform.ErrNoDaemon)
}

func (s suite) socketSubject(t *testing.T) subject {
	t.Helper()
	sub := s.subject(t)
	if sub.socketRoot != nil {
		sub.root = sub.socketRoot(t)
	}
	return sub
}
