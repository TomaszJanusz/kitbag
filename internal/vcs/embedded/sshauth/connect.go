package sshauth

import (
	"context"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

const connectionTimeout = 10 * time.Second

// Connect establishes a context-bound SSH connection for read-only diagnostics.
// Close releases the connection, including when a handshake or exec stalls.
func (a *Auth) Connect() (*ssh.Client, error) {
	config, err := a.ClientConfig()
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(a.Target.Host, strconv.Itoa(a.Target.Port))
	conn, err := (&net.Dialer{Timeout: connectionTimeout}).DialContext(a.ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	a.track(conn)
	if err = a.deadline(conn); err != nil {
		return nil, err
	}
	client, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(client, channels, requests), nil
}

func (a *Auth) deadline(conn net.Conn) error {
	deadline := time.Now().Add(connectionTimeout)
	if bounded, ok := a.ctx.Deadline(); ok && bounded.Before(deadline) {
		deadline = bounded
	}
	return conn.SetDeadline(deadline)
}

func (a *Auth) track(conn net.Conn) {
	a.closers = append(a.closers, conn)
	a.stop = append(a.stop, context.AfterFunc(a.ctx, func() {
		if err := conn.Close(); err != nil {
			return
		}
	}))
}
