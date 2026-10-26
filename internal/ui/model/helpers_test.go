package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// fakeClient scripts every Client call and records writes and syncs.
type fakeClient struct {
	agentsErr   error
	previewErr  error
	applyErr    error
	statusErr   error
	driftErr    error
	settingsErr error
	writeErr    error
	syncErr     error
	subErr      error
	refreshErr  error
	agents      AgentsSnapshot
	preview     AgentPreview
	applied     AgentApplied
	changes     []AgentChange
	revisions   []string
	probes      []bool
	writes      []WriteSetting
	settings    []SettingValue
	phases      []string
	events      []Event
	drift       DriftSnapshot
	status      StatusSnapshot
	outcome     SyncOutcome
	refreshes   int
	syncs       int
	subscribes  int
}

func (f *fakeClient) Status(context.Context) (StatusSnapshot, error) { return f.status, f.statusErr }

func (f *fakeClient) Drift(context.Context) (DriftSnapshot, error) { return f.drift, f.driftErr }

func (f *fakeClient) Sync(_ context.Context, progress func(string)) (SyncOutcome, error) {
	f.syncs++
	for _, phase := range f.phases {
		progress(phase)
	}
	return f.outcome, f.syncErr
}

func (f *fakeClient) Settings(context.Context) ([]SettingValue, error) {
	return f.settings, f.settingsErr
}

func (f *fakeClient) SetSetting(_ context.Context, key, value string) error {
	f.writes = append(f.writes, WriteSetting{Key: key, Value: value})
	return f.writeErr
}

func (f *fakeClient) UnsetSetting(_ context.Context, key string) error {
	f.writes = append(f.writes, WriteSetting{Key: key, Unset: true})
	return f.writeErr
}

func (f *fakeClient) Subscribe(ctx context.Context, deliver func(Event)) error {
	f.subscribes++
	if f.subErr != nil {
		return f.subErr
	}
	deliver(Event{Kind: EventConnected})
	for _, event := range f.events {
		deliver(event)
	}
	return ctx.Err()
}

func (f *fakeClient) Agents(_ context.Context, probe bool) (AgentsSnapshot, error) {
	f.probes = append(f.probes, probe)
	return f.agents, f.agentsErr
}

func (f *fakeClient) PreviewAgentChange(_ context.Context, change AgentChange) (AgentPreview, error) {
	f.changes = append(f.changes, change)
	return f.preview, f.previewErr
}

func (f *fakeClient) ApplyAgentChange(_ context.Context, change AgentChange, revision string) (AgentApplied, error) {
	f.changes, f.revisions = append(f.changes, change), append(f.revisions, revision)
	return f.applied, f.applyErr
}

func fixedNow() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC) }

// execute runs one command synchronously and returns what it emitted.
func execute(t *testing.T, client Client, cmd Cmd) []Msg {
	t.Helper()
	var emitted []Msg
	Runner{Client: client, Now: fixedNow}.Run(t.Context(), cmd, func(msg Msg) { emitted = append(emitted, msg) })
	return emitted
}

type updater interface{ Update(Msg) []Cmd }

// settle executes cmds and feeds results back until no command remains.
// Subscribe commands are not followed, so the loop always terminates.
func settle(t *testing.T, m updater, client Client, cmds []Cmd) {
	t.Helper()
	for len(cmds) > 0 {
		cmd := cmds[0]
		cmds = cmds[1:]
		if _, ok := cmd.(Subscribe); ok {
			continue
		}
		for _, msg := range execute(t, client, cmd) {
			cmds = append(cmds, m.Update(msg)...)
		}
	}
}

// checkText requires every keyed message to exist in the English catalog
// with exactly as many %s verbs as arguments, recursively.
func checkText(t *testing.T, messages ...text.Message) {
	t.Helper()
	catalog := text.English()
	for _, m := range messages {
		if m.Key == "" {
			if len(m.Args) != 0 {
				t.Errorf("plain message with arguments: %+v", m)
			}
			continue
		}
		template, ok := catalog[m.Key]
		if !ok {
			t.Errorf("missing catalog key %q", m.Key)
			continue
		}
		if verbs := strings.Count(template, "%s"); verbs != len(m.Args) {
			t.Errorf("%s has %d verbs but %d arguments", m.Key, verbs, len(m.Args))
		}
		checkText(t, m.Args...)
	}
}

func render(m text.Message) string { return text.English().Render(m) }

func statusTexts(v StatusView) []text.Message {
	messages := []text.Message{v.Title, v.Headline}
	messages = append(messages, v.Notices...)
	messages = append(messages, v.Warnings...)
	if v.Problem != nil {
		messages = append(messages, *v.Problem)
	}
	for _, row := range v.Rows {
		messages = append(messages, row.Label, row.Value)
	}
	for _, action := range v.Actions {
		messages = append(messages, action.Label)
	}
	return messages
}

func action(t *testing.T, controls []Control, intent Msg) Control {
	t.Helper()
	for _, control := range controls {
		if control.Intent == intent {
			return control
		}
	}
	t.Fatalf("no control for %#v", intent)
	return Control{}
}

var errBoom = errors.New("boom")

// expect fails the test unless ok; one condition per call keeps tests flat.
func expect(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

// as performs a checked type assertion.
func as[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	if !ok {
		t.Fatalf("%#v is not %T", value, typed)
	}
	return typed
}

// only returns the single element of values.
func only[T any](t *testing.T, values []T) T {
	t.Helper()
	if len(values) != 1 {
		t.Fatalf("want one element, got %#v", values)
	}
	return values[0]
}

func (f *fakeClient) RefreshService(context.Context) error { f.refreshes++; return f.refreshErr }
