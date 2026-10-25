// Package client calls the local API and falls back only when no daemon listens.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	responseLimit = 8 << 20
	callTimeout   = 30 * time.Second
	requestID     = "1"
)

// Client discovers the socket and reads its token through the platform contract.
type Client struct {
	Host       platform.Host
	APIVersion string
	// KeepAlive spaces Subscribe's keepalive requests; zero uses one minute.
	KeepAlive time.Duration
}

// Call performs one authenticated call, forwarding lifecycle notifications to
// notify when provided. Each call owns its connection; clients may run in parallel.
// A notification callback must return promptly. Authentication/protocol failures
// never fall back to in-process mutation.
func (c Client) Call(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return c.Stream(ctx, method, p, result, notify)
}

// Stream is Call without the fixed 30-second bound, for long-running methods
// such as sync.run; the caller's context alone bounds it.
func (c Client) Stream(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) (err error) {
	conn, p, err := c.open(ctx, p)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	stop := closeOnCancel(ctx, conn)
	defer stop()
	if err = send(conn, method, requestID, p); err != nil {
		return err
	}
	return readResponse(bufio.NewReaderSize(conn, responseLimit), result, notify, p.APIVersion)
}

// open dials the endpoint and completes the authenticated envelope.
func (c Client) open(ctx context.Context, p api.Params) (net.Conn, api.Params, error) {
	path, err := api.SocketPath(c.Host)
	if err != nil {
		return nil, p, err
	}
	conn, err := c.Host.DialLocal(ctx, path)
	if err != nil {
		return nil, p, err
	}
	if p.Token, err = api.LoadToken(c.Host); err != nil {
		return nil, p, errors.Join(err, conn.Close())
	}
	if p.APIVersion == "" {
		p.APIVersion = c.APIVersion
	}
	if p.APIVersion == "" {
		p.APIVersion = api.Version
	}
	return conn, p, nil
}

func closeOnCancel(ctx context.Context, conn net.Conn) func() bool {
	return context.AfterFunc(ctx, func() {
		if closeErr := conn.Close(); closeErr != nil {
			return
		}
	})
}

func send(conn io.Writer, method, id string, p api.Params) error {
	params, err := json.Marshal(p)
	if err != nil {
		return err
	}
	request := api.Request{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method, Params: params}
	return json.NewEncoder(conn).Encode(request)
}

func readResponse(reader *bufio.Reader, result any, notify func(api.Notification), requested string) error {
	for {
		frame, err := reader.ReadSlice('\n')
		if err != nil {
			return fmt.Errorf("local API response: %w", err)
		}
		message, err := parseFrame(frame)
		if err != nil {
			return err
		}
		if message.Method != "" {
			if notify != nil {
				notify(api.Notification{JSONRPC: "2.0", Method: message.Method, Params: message.Params})
			}
			continue
		}
		return decodeResult(message.Response, result, requested)
	}
}

// frame is either a response or a notification; Method distinguishes them.
type frame struct {
	api.Response
	Method string    `json:"method"`
	Params api.Event `json:"params"`
}

func parseFrame(data []byte) (frame, error) {
	var message frame
	if err := json.Unmarshal(data, &message); err != nil || message.JSONRPC != "2.0" {
		return frame{}, errors.New("invalid local API response")
	}
	return message, nil
}

// Status uses the daemon if present, otherwise the supplied core call.
func (c Client) Status(ctx context.Context, options api.Options, local func() (workflow.Status, error)) (workflow.Status, error) {
	var result workflow.Status
	err := c.Call(ctx, "status.get", api.Params{Options: options}, &result, nil)
	if errors.Is(err, platform.ErrNoDaemon) {
		return local()
	}
	return result, err
}

// Inspect uses the daemon if present, otherwise the supplied core call.
func (c Client) Inspect(ctx context.Context, options api.Options, local func() (workflow.Inspection, error)) (workflow.Inspection, error) {
	var result api.PlanResult
	err := c.Call(ctx, "plan.get", api.Params{Options: options}, &result, nil)
	if errors.Is(err, platform.ErrNoDaemon) {
		return local()
	}
	return workflow.Inspection{Plan: result.Plan, Warnings: result.Warnings}, err
}

func decodeResult(response api.Response, result any, requested string) error {
	if string(response.ID) != requestID || (response.Error == nil) == (response.Result == nil) {
		return errors.New("invalid local API response")
	}
	if response.Error != nil {
		return response.Error
	}
	if !compatible(requested, response.Result.APIVersion) {
		return errors.New("incompatible local API response")
	}
	return json.Unmarshal(response.Result.Data, result)
}

func compatible(requested, selected string) bool {
	major, minor, ok := strings.Cut(selected, ".")
	wantMajor, wantMinor, wantOK := strings.Cut(requested, ".")
	if !ok || !wantOK || major != "1" || major != wantMajor || minor == "" || strings.Trim(minor, "0123456789") != "" {
		return false
	}
	if len(minor) > 1 && minor[0] == '0' {
		return false
	}
	if len(minor) != len(wantMinor) {
		return len(minor) < len(wantMinor)
	}
	return minor <= wantMinor
}
