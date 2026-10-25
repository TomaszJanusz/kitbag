package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
)

const (
	// keepAlive stays below the server's two-minute read-idle limit.
	keepAlive   = time.Minute
	keepAliveID = "2"
)

// Subscribe registers one connection for lifecycle hints. ready runs once the
// server confirms; notify then receives each hint until ctx ends or the
// connection fails, and Subscribe returns that cause. Periodic version.get
// requests keep an otherwise silent connection open. Hints may be lost, so
// callers refresh snapshots after (re)subscribing. Callbacks must return promptly.
func (c Client) Subscribe(ctx context.Context, options api.Options, ready func(), notify func(api.Notification)) error {
	conn, p, err := c.open(ctx, api.Params{Options: options})
	if err != nil {
		return err
	}
	closeConn := sync.OnceFunc(func() { closeQuiet(conn) })
	defer closeConn()
	stop := context.AfterFunc(ctx, closeConn)
	defer stop()
	reader := bufio.NewReaderSize(conn, responseLimit)
	if err = c.confirm(ctx, conn, reader, p, notify); err != nil {
		return cause(ctx, err)
	}
	ready()
	done := make(chan struct{})
	var workers sync.WaitGroup
	workers.Go(func() { c.keepAlive(conn, p, done) })
	err = listen(reader, notify)
	closeConn()
	close(done)
	workers.Wait()
	return cause(ctx, err)
}

func (c Client) confirm(ctx context.Context, conn net.Conn, reader *bufio.Reader, p api.Params, notify func(api.Notification)) error {
	if err := conn.SetReadDeadline(time.Now().Add(callTimeout)); err != nil {
		return err
	}
	if err := send(conn, "events.subscribe", requestID, p); err != nil {
		return err
	}
	var result struct {
		Subscribed bool `json:"subscribed"`
	}
	if err := readResponse(reader, &result, notify, p.APIVersion); err != nil {
		return err
	}
	if !result.Subscribed {
		return errors.New("invalid local API response")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return conn.SetReadDeadline(time.Time{})
}

func (c Client) keepAlive(conn io.Writer, p api.Params, done <-chan struct{}) {
	interval := c.KeepAlive
	if interval <= 0 {
		interval = keepAlive
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if send(conn, "version.get", keepAliveID, p) != nil {
				return
			}
		}
	}
}

// listen forwards notifications and discards keepalive results. A rejected
// keepalive, for example after token rotation, ends the subscription.
func listen(reader *bufio.Reader, notify func(api.Notification)) error {
	for {
		data, err := reader.ReadSlice('\n')
		if err != nil {
			return fmt.Errorf("local API subscription: %w", err)
		}
		message, err := parseFrame(data)
		if err != nil {
			return err
		}
		if message.Method != "" {
			if notify != nil {
				notify(api.Notification{JSONRPC: "2.0", Method: message.Method, Params: message.Params})
			}
			continue
		}
		if message.Error != nil {
			return message.Error
		}
	}
}

// cause reports cancellation rather than the closed connection it produced.
func cause(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func closeQuiet(closer io.Closer) {
	// The subscription has ended; a close failure carries no further information.
	if err := closer.Close(); err != nil {
		return
	}
}
