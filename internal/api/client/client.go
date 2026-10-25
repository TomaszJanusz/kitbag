// Package client calls the local API and falls back only when no daemon listens.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	responseLimit = 8 << 20
	callTimeout   = 30 * time.Second
)

// Client discovers the socket and reads its token through the platform contract.
type Client struct {
	Host       platform.Host
	APIVersion string
}

// Call performs one authenticated call, forwarding lifecycle notifications to
// notify when provided. Each call owns its connection; clients may run in parallel.
// A notification callback must return promptly. Authentication/protocol failures
// never fall back to in-process mutation.
func (c Client) Call(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) (err error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	path, err := api.SocketPath(c.Host)
	if err != nil {
		return err
	}
	conn, err := c.Host.DialLocal(ctx, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	stop := context.AfterFunc(ctx, func() {
		if closeErr := conn.Close(); closeErr != nil {
			return
		}
	})
	defer stop()
	p.Token, err = api.LoadToken(c.Host)
	if err != nil {
		return err
	}
	if p.APIVersion == "" {
		p.APIVersion = c.APIVersion
	}
	if p.APIVersion == "" {
		p.APIVersion = api.Version
	}
	params, err := json.Marshal(p)
	if err != nil {
		return err
	}
	request := api.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: method, Params: params}
	if err = json.NewEncoder(conn).Encode(request); err != nil {
		return err
	}
	return readResponse(conn, result, notify, p.APIVersion)
}

func readResponse(source io.Reader, result any, notify func(api.Notification), requested string) error {
	reader := bufio.NewReaderSize(source, responseLimit)
	for {
		frame, err := reader.ReadSlice('\n')
		if err != nil {
			return fmt.Errorf("local API response: %w", err)
		}
		var envelope struct {
			api.Response
			Method string    `json:"method"`
			Params api.Event `json:"params"`
		}
		if err = json.Unmarshal(frame, &envelope); err != nil || envelope.JSONRPC != "2.0" {
			return errors.New("invalid local API response")
		}
		if envelope.Method != "" {
			if notify != nil {
				notify(api.Notification{JSONRPC: "2.0", Method: envelope.Method, Params: envelope.Params})
			}
			continue
		}
		return decodeResult(envelope.Response, result, requested)
	}
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
	if string(response.ID) != "1" || (response.Error == nil) == (response.Result == nil) {
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
