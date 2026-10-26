// Package apiclient adapts the local API to the shared view models' Client.
// Like kitbag status and plan, it falls back to the same in-process core
// services only when no daemon listens; authentication, protocol and lock
// failures are reported, never retried locally.
package apiclient

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/settings"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// Transport is the subset of internal/api/client.Client used here.
type Transport interface {
	Call(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) error
	Stream(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) error
	Subscribe(ctx context.Context, options api.Options, ready func(), notify func(api.Notification)) error
}

// Local builds the CLI's in-process services for the no-daemon fallback.
// A nil factory reports the daemon as unavailable instead.
type Local struct {
	Workflow func(context.Context) (workflow.Service, error)
	Settings func() (settings.Service, error)
}

// Client implements model.Client. Env and Flags resolve setting overrides
// for this process exactly as kitbag config list does; Options forwards the
// explicit selections the CLI forwards to the daemon.
type Client struct {
	Transport Transport
	Env       platform.Env
	Flags     map[string]string
	Local     Local
	Options   api.Options
}

var _ model.Client = Client{}

func (c Client) params() api.Params { return api.Params{Options: c.Options} }

// noDaemon reports whether err permits the in-process fallback.
func noDaemon(err error) bool { return errors.Is(err, platform.ErrNoDaemon) }

// Status reads status.get, falling back to workflow.Service.Status.
func (c Client) Status(ctx context.Context) (model.StatusSnapshot, error) {
	var result workflow.Status
	err := c.Transport.Call(ctx, "status.get", c.params(), &result, nil)
	daemon := true
	if noDaemon(err) && c.Local.Workflow != nil {
		daemon = false
		result, err = c.localStatus(ctx)
	}
	return statusSnapshot(result, daemon), classify(err)
}

func (c Client) localStatus(ctx context.Context) (workflow.Status, error) {
	service, err := c.Local.Workflow(ctx)
	if err != nil {
		return workflow.Status{ToolTooOld: compatibility.FromError(err, "")}, err
	}
	return service.Status(ctx)
}

// Drift reads plan.get, which is drift.list plus the planned operations,
// falling back to workflow.Service.Inspect.
func (c Client) Drift(ctx context.Context) (model.DriftSnapshot, error) {
	var result api.PlanResult
	err := c.Transport.Call(ctx, "plan.get", c.params(), &result, nil)
	daemon := true
	if noDaemon(err) && c.Local.Workflow != nil {
		daemon = false
		var service workflow.Service
		if service, err = c.Local.Workflow(ctx); err == nil {
			var inspection workflow.Inspection
			inspection, err = service.Inspect()
			result = api.PlanResult{Plan: inspection.Plan, Warnings: inspection.Warnings}
		}
	}
	if err != nil {
		return model.DriftSnapshot{Daemon: daemon}, classify(err)
	}
	return driftSnapshot(result, daemon), nil
}

// Sync runs sync.run without the ordinary call deadline, forwarding progress,
// or runs workflow.Service.Sync in-process with the same phases.
func (c Client) Sync(ctx context.Context, progress func(string)) (model.SyncOutcome, error) {
	var result api.SyncResult
	err := c.Transport.Stream(ctx, "sync.run", c.params(), &result, func(n api.Notification) {
		if n.Method == "sync.progress" {
			progress(n.Params.Phase)
		}
	})
	if noDaemon(err) && c.Local.Workflow != nil {
		return c.localSync(ctx, progress)
	}
	if err != nil {
		var remote *api.Error
		code := int(apply.ErrorCode(err))
		if errors.As(err, &remote) {
			code = int(remote.ExitCode())
		}
		return model.SyncOutcome{ExitCode: code}, classify(err)
	}
	return model.SyncOutcome{ExitCode: int(result.ExitCode), Changed: result.Changed}, nil
}

func (c Client) localSync(ctx context.Context, progress func(string)) (model.SyncOutcome, error) {
	service, err := c.Local.Workflow(ctx)
	if err != nil {
		return model.SyncOutcome{ExitCode: int(apply.ErrorCode(err))}, classify(err)
	}
	progress("started")
	service.Progress = progress
	result, err := service.Sync(ctx, false, false)
	outcome := model.SyncOutcome{ExitCode: int(max(result.Code(), apply.ErrorCode(err))), Changed: len(result.Apply.Changed) + len(result.Includes.Added)}
	progress("finished")
	return outcome, classify(err)
}

// Settings resolves config.get bytes with this process's overrides, or lists
// the local settings service.
func (c Client) Settings(ctx context.Context) ([]model.SettingValue, error) {
	var result api.ConfigResult
	err := c.Transport.Call(ctx, "config.get", c.params(), &result, nil)
	var entries []settings.Entry
	switch {
	case noDaemon(err) && c.Local.Settings != nil:
		entries, err = c.localSettings()
	case err == nil && c.Env == nil:
		err = fmt.Errorf("settings environment: %w", fs.ErrInvalid)
	case err == nil:
		entries, err = settings.Entries(c.Env, []byte(result.Config), c.Flags)
	}
	if err != nil {
		return nil, classify(err)
	}
	return settingValues(entries), nil
}

func (c Client) localSettings() ([]settings.Entry, error) {
	service, err := c.Local.Settings()
	if err != nil {
		return nil, err
	}
	return service.List()
}

// SetSetting stores a validated value.
func (c Client) SetSetting(ctx context.Context, key, value string) error {
	return c.change(ctx, key, value, false)
}

// UnsetSetting removes a stored value.
func (c Client) UnsetSetting(ctx context.Context, key string) error {
	return c.change(ctx, key, "", true)
}

// change edits the daemon's file as config.get, settings.Changed and
// config.set. The API replaces whole files without compare-and-swap, so a
// concurrent edit between the two calls is overwritten (documented API gap).
func (c Client) change(ctx context.Context, key, value string, unset bool) error {
	var current api.ConfigResult
	err := c.Transport.Call(ctx, "config.get", c.params(), &current, nil)
	if noDaemon(err) && c.Local.Settings != nil {
		var service settings.Service
		if service, err = c.Local.Settings(); err == nil {
			err = service.Change(key, value, unset)
		}
		return classify(err)
	}
	if err != nil {
		return classify(err)
	}
	candidate, err := settings.Changed([]byte(current.Config), key, value, unset)
	if err != nil {
		return classify(err)
	}
	p := c.params()
	p.Config = string(candidate)
	return classify(c.Transport.Call(ctx, "config.set", p, &current, nil))
}

// Subscribe forwards lifecycle hints. There is no in-process equivalent.
func (c Client) Subscribe(ctx context.Context, deliver func(model.Event)) error {
	return classify(c.Transport.Subscribe(ctx, c.Options,
		func() { deliver(model.Event{Kind: model.EventConnected}) },
		func(n api.Notification) {
			deliver(model.Event{Kind: model.EventKind(n.Method), ExitCode: n.Params.ExitCode, Changed: n.Params.Changed})
		}))
}

func statusSnapshot(s workflow.Status, daemon bool) model.StatusSnapshot {
	snapshot := model.StatusSnapshot{
		Repository: s.Repository, Warnings: s.Warnings, Daemon: daemon, LastSync: lastSync(s.LastSync),
		Git: model.GitState{
			Branch: s.Git.Branch, Commit: s.Git.Commit, Upstream: s.Git.Upstream, Ahead: s.Git.Ahead,
			Behind: s.Git.Behind, Dirty: s.Git.Dirty, UpstreamGone: s.Git.UpstreamGone,
		},
		Drift: model.DriftCounts{
			InSync: s.Drift[plan.InSync], UpdateNeeded: s.Drift[plan.UpdateNeeded], LocalDrift: s.Drift[plan.LocalDrift],
			Conflict: s.Drift[plan.Conflict], UnmanagedObstacle: s.Drift[plan.UnmanagedObstacle],
		},
	}
	if s.ToolTooOld != nil {
		snapshot.ToolTooOld = &model.ToolTooOld{
			Required: s.ToolTooOld.Required, Current: s.ToolTooOld.Current,
			Schema: s.ToolTooOld.Schema, SupportedSchema: s.ToolTooOld.SupportedSchema,
		}
	}
	return snapshot
}

func lastSync(record *state.SyncRecord) *model.LastSync {
	if record == nil {
		return nil
	}
	return &model.LastSync{At: record.At, ExitCode: record.Result, Changed: record.Changed}
}

// driftSnapshot keeps drift.list's selection (every observation not in sync)
// and attaches the planned operations for each path.
func driftSnapshot(result api.PlanResult, daemon bool) model.DriftSnapshot {
	actions := map[string][]model.Action{}
	for _, op := range result.Plan.Operations {
		action := model.Action{Kind: model.ActionKind(op.Kind), Warning: op.Warning, BackupPath: op.BackupPath}
		if op.Desired != nil {
			action.SourcePath = op.Desired.SourcePath
		}
		actions[op.Path] = append(actions[op.Path], action)
	}
	snapshot := model.DriftSnapshot{Warnings: result.Warnings, Daemon: daemon}
	for _, observation := range result.Plan.Observations {
		if observation.Classification == plan.InSync {
			continue
		}
		snapshot.Items = append(snapshot.Items, model.DriftItem{
			Path: observation.Path, Classification: model.Classification(observation.Classification), Actions: actions[observation.Path],
		})
	}
	return snapshot
}

func settingValues(entries []settings.Entry) []model.SettingValue {
	values := make([]model.SettingValue, 0, len(entries))
	for _, entry := range entries {
		value := model.SettingValue{Key: entry.Key.Path, Value: entry.Value, Name: entry.Source}
		switch entry.Source {
		case "default":
			value.Source, value.Name = model.SourceDefault, ""
		case "--" + entry.Key.Flag:
			value.Source = model.SourceFlag
		case entry.Key.Env:
			value.Source = model.SourceEnvironment
		default:
			value.Source, value.Name = model.SourceFile, ""
		}
		values = append(values, value)
	}
	return values
}
