// Package api serves the versioned local JSON-RPC contract. Synchronization
// policy remains in core; this package authenticates, dispatches and frames it.
package api

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
)

// Protocol and resource limits are part of the published v1 contract.
const (
	Version            = "1.0"
	MaxRequestBytes    = 1 << 20
	EventBuffer        = 32
	MaxConnections     = 32
	ParseError         = -32700
	InvalidRequest     = -32600
	MethodNotFound     = -32601
	InvalidParams      = -32602
	InternalError      = -32603
	Unauthorized       = -32000
	UnsupportedVersion = -32001
	OperationFailed    = -32002
	Busy               = -32003
	RequestTooLarge    = -32004
)

// Options preserves the CLI's explicit machine selections. Unset values use
// the serving process's environment and the selected configuration file.
type Options struct {
	Config     string `json:"config,omitempty"`
	Host       string `json:"host,omitempty"`
	Mode       string `json:"mode,omitempty"`
	GitBackend string `json:"git_backend,omitempty"`
}

// Params contains the common authenticated envelope and method arguments.
// Unknown members are optional extensions and are ignored within major v1.
type Params struct {
	Token      string  `json:"token"`
	APIVersion string  `json:"api_version"`
	Options    Options `json:"options,omitempty"`
	Config     string  `json:"config,omitempty"`
	NoPull     bool    `json:"no_pull,omitempty"`
	Force      bool    `json:"force,omitempty"`
}

// Request is a JSON-RPC 2.0 request. Omit ID for a notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// ErrorData maps errors to CLI exits without exposing diagnostics or secrets.
type ErrorData struct {
	MinVersion string     `json:"min_version"`
	MaxVersion string     `json:"max_version"`
	ExitCode   apply.Code `json:"exit_code"`
}

// Error is a stable, sanitized JSON-RPC error.
type Error struct {
	Message string    `json:"message"`
	Data    ErrorData `json:"data"`
	Code    int       `json:"code"`
}

// Error returns only the public description, never the request or cause.
func (e *Error) Error() string { return e.Message }

// ExitCode returns the CLI classification.
func (e *Error) ExitCode() apply.Code { return e.Data.ExitCode }

// Result wraps method data with the selected compatible API version.
type Result struct {
	APIVersion string          `json:"api_version"`
	Data       json.RawMessage `json:"data"`
}

// Response is exactly one result or error and the matching request ID.
type Response struct {
	Result  *Result         `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
}

// Notification contains no ID and can be interleaved with responses.
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  Event  `json:"params"`
}

func rpcError(code int, message string, exit apply.Code) *Error {
	return &Error{Code: code, Message: message, Data: ErrorData{ExitCode: exit, MinVersion: Version, MaxVersion: Version}}
}

func negotiate(version string) error {
	major, minor, found := strings.Cut(version, ".")
	if !found || major != "1" || minor == "" || strings.Trim(minor, "0123456789") != "" {
		return errors.New("unsupported API version")
	}
	if len(minor) > 1 && minor[0] == '0' {
		return errors.New("unsupported API version")
	}
	return nil
}
