package sshauth

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDiagnosticHandshakeDeadline(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				return
			}
		}()
		if _, copyErr := io.Copy(io.Discard, conn); copyErr != nil {
			return
		}
	}()
	t.Cleanup(func() { must(t, listener.Close()); <-done })
	host, port, err := net.SplitHostPort(listener.Addr().String())
	must(t, err)
	portNumber, err := strconv.Atoi(port)
	must(t, err)
	const timeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	auth, err := New(ctx, Target{Host: host, Port: portNumber, User: "git", KnownHosts: []string{filepath.Join(t.TempDir(), "missing")}}, Options{FS: fixtureFS{}})
	must(t, err)
	defer func() { must(t, auth.Close()) }()
	if _, err = auth.Connect(); err == nil {
		t.Fatal("stalled handshake authenticated")
	}
}
