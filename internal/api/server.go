package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"sync"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
)

const (
	idleTimeout  = 2 * time.Minute
	writeTimeout = 2 * time.Second
)

// Serve owns listener until cancellation or an accept error. It bounds active
// connections, request frames and notification buffers. Cancellation closes all
// connections and waits for handlers; factories must honor their context.
func Serve(ctx context.Context, listener net.Listener, handlers Handlers) error {
	if listener == nil || !validHandlers(handlers) {
		return fs.ErrInvalid
	}
	defer closeQuiet(listener)
	if handlers.Events == nil {
		handlers.Events = NewEvents()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { closeQuiet(listener) })
	defer stop()
	slots := make(chan struct{}, MaxConnections)
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			workers.Go(func() { defer func() { <-slots }(); serveConnection(ctx, conn, handlers) })
		default:
			closeQuiet(conn)
		}
	}
}

type outgoing struct {
	value any
	done  chan struct{}
}

type connection struct {
	conn     net.Conn
	out      chan outgoing
	handlers Handlers
}

func serveConnection(ctx context.Context, conn net.Conn, h Handlers) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer closeQuiet(conn)
	stop := context.AfterFunc(ctx, func() { closeQuiet(conn) })
	defer stop()
	c := &connection{conn: conn, handlers: h, out: make(chan outgoing, EventBuffer)}
	var workers sync.WaitGroup
	workers.Go(func() { c.write(ctx, cancel) })
	defer workers.Wait()
	defer cancel()
	c.read(ctx)
}

func (c *connection) write(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	encoder := json.NewEncoder(c.conn)
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-c.out:
			if c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)) != nil {
				return
			}
			if encoder.Encode(message.value) != nil {
				return
			}
			if message.done != nil {
				close(message.done)
			}
		}
	}
}

func (c *connection) read(ctx context.Context) {
	reader := bufio.NewReaderSize(c.conn, MaxRequestBytes)
	unsubscribe := func() {}
	defer func() { unsubscribe() }()
	for {
		if c.conn.SetReadDeadline(time.Now().Add(idleTimeout)) != nil {
			return
		}
		frame, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			c.send(ctx, failure(nil, rpcError(RequestTooLarge, "request too large", apply.Usage)))
			return
		}
		if err != nil && len(frame) == 0 {
			return
		}
		request, problem := parseRequest(frame)
		if problem != nil {
			c.send(ctx, failure(nil, problem))
			continue
		}
		p, problem := c.authenticate(request)
		if problem != nil {
			c.reply(ctx, request, nil, problem)
			continue
		}
		if request.Method == "events.subscribe" {
			unsubscribe()
			unsubscribe = c.subscribe(ctx)
			c.reply(ctx, request, map[string]bool{"subscribed": true}, nil)
		} else {
			result, callErr := c.handlers.call(ctx, request.Method, p, request.ID, c.notify)
			c.reply(ctx, request, result, callErr)
		}
	}
}

func parseRequest(frame []byte) (Request, *Error) {
	var request Request
	if !json.Valid(frame) {
		return request, rpcError(ParseError, "parse error", apply.Usage)
	}
	if err := json.Unmarshal(frame, &request); err != nil || request.JSONRPC != "2.0" || request.Method == "" || !validID(request.ID) {
		return Request{}, rpcError(InvalidRequest, "invalid request", apply.Usage)
	}
	return request, nil
}

func validID(id json.RawMessage) bool {
	if len(id) == 0 || string(id) == "null" {
		return true
	}
	var value any
	if json.Unmarshal(id, &value) != nil {
		return false
	}
	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}

func (c *connection) authenticate(request Request) (Params, *Error) {
	var p Params
	if len(request.Params) == 0 || json.Unmarshal(request.Params, &p) != nil || string(request.Params) == "null" {
		return p, rpcError(InvalidParams, "invalid params", apply.Usage)
	}
	if !authorized(c.handlers.Token, p.Token) {
		return p, rpcError(Unauthorized, "unauthorized", apply.Usage)
	}
	if negotiate(p.APIVersion) != nil {
		return p, rpcError(UnsupportedVersion, "unsupported API version", apply.Usage)
	}
	return p, nil
}

func failure(id json.RawMessage, problem *Error) Response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return Response{JSONRPC: "2.0", ID: id, Error: problem}
}

func (c *connection) reply(ctx context.Context, request Request, result any, problem *Error) {
	if len(request.ID) == 0 {
		return
	}
	if problem != nil {
		c.send(ctx, failure(request.ID, problem))
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		c.send(ctx, failure(request.ID, rpcError(InternalError, "internal error", apply.Warnings)))
		return
	}
	c.send(ctx, Response{JSONRPC: "2.0", ID: request.ID, Result: &Result{APIVersion: Version, Data: data}})
}

func (c *connection) send(ctx context.Context, message any) {
	done := make(chan struct{})
	select {
	case c.out <- outgoing{value: message, done: done}:
	case <-ctx.Done():
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (c *connection) notify(event Event) {
	event.APIVersion = Version
	select {
	case c.out <- outgoing{value: Notification{JSONRPC: "2.0", Method: event.Kind, Params: event}}:
	default:
	}
}

func (c *connection) subscribe(ctx context.Context) func() {
	queue, unsubscribe := c.handlers.Events.subscribe()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer unsubscribe()
		for {
			select {
			case event := <-queue:
				c.notify(event)
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}

func closeQuiet(closer io.Closer) {
	// Shutdown is best effort. Errors carry no actionable response after closure.
	if err := closer.Close(); err != nil {
		return
	}
}

func validHandlers(h Handlers) bool {
	return len(h.Token) == tokenLength && h.Host != nil && h.Service != nil
}
