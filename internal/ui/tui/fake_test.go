package tui

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/settings"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// fakeClient is a scripted model.Client. Sync reports each phase sent on
// phases and finishes when phases is closed; Subscribe delivers events until
// its context ends.
type fakeClient struct {
	statusErr    error
	settingErr   error
	configData   string
	phases       chan string
	events       chan model.Event
	writes       chan [2]string
	settings     []model.SettingValue
	drift        model.DriftSnapshot
	status       model.StatusSnapshot
	mu           sync.Mutex
	delay        time.Duration
	inflight     atomic.Int32
	overlapped   atomic.Bool
	refreshes    atomic.Int32
	statusCalls  atomic.Int32
	subscribed   atomic.Int32
	unsubscribed atomic.Int32
	panicStatus  bool
}

func newFake() *fakeClient {
	return &fakeClient{
		phases: make(chan string), events: make(chan model.Event), writes: make(chan [2]string, 8),
		status: model.StatusSnapshot{
			Repository: "/home/user/.local/share/kitbag/repo",
			Git:        model.GitState{Branch: "main", Upstream: "origin/main", Behind: 1},
			LastSync:   &model.LastSync{At: time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC), Changed: 2},
			Drift:      model.DriftCounts{InSync: 5, UpdateNeeded: 1, LocalDrift: 2},
			Warnings:   []string{"kitbag.toml: unknown agent \"zed\" ignored"},
			Daemon:     true,
		},
		drift: model.DriftSnapshot{Daemon: true, Items: []model.DriftItem{
			{Path: "/home/user/.claude/CLAUDE.md", Classification: model.UpdateNeeded, Actions: []model.Action{{Kind: model.ActionWrite, SourcePath: "agents/CLAUDE.md"}}},
			{Path: "/home/user/.codex/AGENTS.md", Classification: model.LocalDrift, Actions: []model.Action{
				{Kind: model.ActionBackup, BackupPath: "/home/user/.local/state/kitbag/backup/AGENTS.md"}, {Kind: model.ActionMaterialize},
			}},
		}},
		settings: []model.SettingValue{
			{Key: "host", Value: "laptop", Source: model.SourceFile, Hosts: []string{"laptop", "mac"}},
			{Key: "repo.url", Source: model.SourceDefault},
			{Key: "git.backend", Value: "auto", Source: model.SourceDefault},
			{Key: "sync.poll_interval", Value: "60s", Source: model.SourceDefault},
			{Key: "daemon.update_check", Value: "true", Source: model.SourceDefault},
			{Key: "daemon.debounce", Value: "500ms", Source: model.SourceDefault},
			{Key: "ui.color", Value: "auto", Source: model.SourceDefault},
			{Key: "sync.mode", Value: "copy", Name: "KITBAG_MODE", Source: model.SourceEnvironment},
		},
	}
}

// enter records calls that overlap, like a contended state lock would.
func (f *fakeClient) enter() func() {
	if f.inflight.Add(1) > 1 {
		f.overlapped.Store(true)
	}
	time.Sleep(f.delay)
	return func() { f.inflight.Add(-1) }
}

func (f *fakeClient) Status(context.Context) (model.StatusSnapshot, error) {
	defer f.enter()()
	f.statusCalls.Add(1)
	if f.panicStatus {
		panic("scripted status failure")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, f.statusErr
}

func (f *fakeClient) Drift(context.Context) (model.DriftSnapshot, error) {
	defer f.enter()()
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.drift, nil
}

func (f *fakeClient) Sync(ctx context.Context, progress func(string)) (model.SyncOutcome, error) {
	defer f.enter()()
	for {
		select {
		case phase, ok := <-f.phases:
			if !ok {
				return model.SyncOutcome{Changed: 3}, nil
			}
			progress(phase)
		case <-ctx.Done():
			return model.SyncOutcome{}, ctx.Err()
		}
	}
}

func (f *fakeClient) Settings(context.Context) ([]model.SettingValue, error) {
	defer f.enter()()
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.SettingValue(nil), f.settings...), nil
}

func (f *fakeClient) SetSetting(_ context.Context, key, value string) error {
	defer f.enter()()
	f.writes <- [2]string{key, value}
	if f.configData != "" {
		if _, err := settings.Changed([]byte(f.configData), key, value, false); err != nil {
			return &model.Error{Kind: model.ErrInvalid, Err: err}
		}
	}
	if f.settingErr != nil {
		return f.settingErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.settings {
		if f.settings[i].Key == key {
			f.settings[i].Value, f.settings[i].Source = value, model.SourceFile
		}
	}
	return nil
}

func (f *fakeClient) UnsetSetting(_ context.Context, key string) error {
	f.writes <- [2]string{key, ""}
	return nil
}

func (f *fakeClient) Subscribe(ctx context.Context, deliver func(model.Event)) error {
	f.subscribed.Add(1)
	defer f.unsubscribed.Add(1)
	deliver(model.Event{Kind: model.EventConnected})
	for {
		select {
		case event := <-f.events:
			deliver(event)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (f *fakeClient) RefreshService(context.Context) error { f.refreshes.Add(1); return nil }
