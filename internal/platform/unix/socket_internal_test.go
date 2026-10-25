//go:build darwin || linux

package unix

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSocketRejectsForeignPeer(t *testing.T) {
	t.Parallel()
	host := New(WithEnv(func(string) (string, bool) { return "", false }))
	path := peerSocketPath(t)
	original, err := host.ListenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	listener, ok := original.(*localListener)
	if !ok {
		t.Fatal("unexpected listener type")
	}
	// Inject the kernel's credential-check result at the platform boundary.
	// The first peer is foreign, the next is checked by the real getsockopt.
	attempts := 0
	listener.verify = func(conn net.Conn) error {
		attempts++
		if attempts == 1 {
			return fs.ErrPermission
		}
		return verifyPeer(conn)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	done := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			done <- acceptErr
			return
		}
		done <- conn.Close()
	}()
	checkForeignSocket(t, host, path)
	owner, err := host.DialLocal(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func checkForeignSocket(t *testing.T, host *Host, path string) {
	t.Helper()
	foreign, err := host.DialLocal(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err = foreign.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	_, err = foreign.Read(buffer)
	if !errors.Is(err, io.EOF) {
		t.Errorf("foreign peer read: %v, want EOF", err)
	}
	if err = foreign.Close(); err != nil {
		t.Fatal(err)
	}
}

func peerSocketPath(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "kitbag-peer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			t.Error(removeErr)
		}
	})
	return filepath.Join(root, "run", "api.sock")
}
