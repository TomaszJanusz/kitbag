package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/initialize"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/settings"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// ServiceFactory builds the same configured core service used by the CLI.
// It must be safe for concurrent calls and honor every supplied selection.
type ServiceFactory func(context.Context, Options) (workflow.Service, error)

// Handlers supplies authentication and dependencies. KIT-30 owns daemon lifetime.
// Configuration is loaded afresh for every call by Service; no snapshot is cached.
type Handlers struct {
	Check       func(string) *state.CheckRecord
	Host        platform.Host
	Service     ServiceFactory
	Events      *Events
	Token       string
	ToolVersion string
}

// VersionInfo advertises the compatible range and available methods.
type VersionInfo struct {
	MinVersion  string   `json:"min_version"`
	MaxVersion  string   `json:"max_version"`
	ToolVersion string   `json:"tool_version"`
	Methods     []string `json:"methods"`
}

// PlanResult contains the safe plan projection and adapter warnings.
type PlanResult struct {
	Warnings []string  `json:"warnings"`
	Plan     plan.Plan `json:"plan"`
}

// SyncResult reports the shared command exit code and completed operation count.
type SyncResult struct {
	ExitCode apply.Code `json:"exit_code"`
	Changed  int        `json:"changed"`
}

// ConfigResult returns validated machine TOML without interpreting agent files.
type ConfigResult struct {
	Config string `json:"config"`
}

func methods() []string {
	return []string{"status.get", "plan.get", "sync.run", "drift.list", "config.get", "config.set", "events.subscribe", "version.get"}
}

func (h Handlers) call(ctx context.Context, method string, p Params, id json.RawMessage, notify func(Event)) (any, *Error) {
	switch method {
	case "version.get":
		return VersionInfo{MinVersion: Version, MaxVersion: Version, ToolVersion: h.ToolVersion, Methods: methods()}, nil
	case "config.get", "config.set":
		return h.configuration(method, p)
	case "status.get", "plan.get", "sync.run", "drift.list":
		return h.workflow(ctx, method, p, id, notify)
	default:
		return nil, rpcError(MethodNotFound, "method not found", apply.Usage)
	}
}

func (h Handlers) workflow(ctx context.Context, method string, p Params, id json.RawMessage, notify func(Event)) (any, *Error) {
	service, err := h.Service(ctx, p.Options)
	if err != nil {
		return nil, operationError(err)
	}
	switch method {
	case "status.get":
		result, err := service.Status(ctx)
		result.DaemonVersion = h.ToolVersion
		result.DaemonRunning = true
		if h.Check != nil {
			result.LastCheck = h.Check(service.Repository)
		}
		return result, operationError(err)
	case "plan.get", "drift.list":
		result, err := service.Inspect()
		if err != nil {
			return nil, operationError(err)
		}
		if method == "drift.list" {
			return drift(result.Plan), nil
		}
		return PlanResult{Plan: result.Plan, Warnings: result.Warnings}, nil
	case "sync.run":
		return h.sync(ctx, service, p, id, notify)
	default:
		return nil, rpcError(MethodNotFound, "method not found", apply.Usage)
	}
}

func drift(p plan.Plan) []plan.Observation {
	result := make([]plan.Observation, 0, len(p.Observations))
	for _, observation := range p.Observations {
		if observation.Classification != plan.InSync {
			result = append(result, observation)
		}
	}
	return result
}

func (h Handlers) sync(ctx context.Context, service workflow.Service, p Params, id json.RawMessage, notify func(Event)) (any, *Error) {
	started := Event{DaemonVersion: h.ToolVersion, Kind: "sync.started", RequestID: id}
	h.Events.Publish(started)
	notify(Event{DaemonVersion: h.ToolVersion, Kind: "sync.progress", Phase: "started", RequestID: id})
	service.Progress = func(phase string) {
		notify(Event{DaemonVersion: h.ToolVersion, Kind: "sync.progress", Phase: phase, RequestID: id})
	}
	result, err := service.Sync(ctx, p.NoPull, p.Force)
	code := max(result.Code(), apply.ErrorCode(err))
	changed := len(result.Apply.Changed) + len(result.Includes.Added)
	h.Events.Publish(Event{DaemonVersion: h.ToolVersion, Kind: "sync.finished", RequestID: id, ExitCode: int(code), Changed: changed})
	notify(Event{DaemonVersion: h.ToolVersion, Kind: "sync.progress", Phase: "finished", RequestID: id, ExitCode: int(code), Changed: changed})
	if err != nil {
		problem := operationError(err)
		problem.Data.ExitCode = code
		return nil, problem
	}
	return SyncResult{ExitCode: code, Changed: changed}, nil
}

func (h Handlers) configuration(method string, p Params) (any, *Error) {
	filename, err := initialize.ConfigPath(h.Host, p.Options.Config)
	if err != nil {
		return nil, operationError(err)
	}
	service := settings.Service{Host: h.Host, Filename: filename}
	if method == "config.set" {
		if err = service.Replace([]byte(p.Config)); err != nil {
			return nil, operationError(err)
		}
	}
	data, _, err := service.Read()
	if err != nil {
		return nil, operationError(err)
	}
	return ConfigResult{Config: string(data)}, nil
}

func operationError(err error) *Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, platform.ErrLocked) {
		return rpcError(Busy, "another operation is running", apply.Warnings)
	}
	code := apply.ErrorCode(err)
	if errors.Is(err, fs.ErrInvalid) {
		code = apply.Usage
	}
	return rpcError(OperationFailed, "operation failed", code)
}
