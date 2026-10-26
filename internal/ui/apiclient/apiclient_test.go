package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/settings"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type call struct {
	method string
	params api.Params
}

// fakeTransport answers each method with a JSON value or an error.
type fakeTransport struct {
	results map[string]any
	errs    map[string]error
	subErr  error
	calls   []call
	notes   []api.Notification
}

func (f *fakeTransport) Call(_ context.Context, method string, p api.Params, result any, notify func(api.Notification)) error {
	f.calls = append(f.calls, call{method: method, params: p})
	for _, note := range f.notes {
		if notify != nil {
			notify(note)
		}
	}
	if err := f.errs[method]; err != nil {
		return err
	}
	data, err := json.Marshal(f.results[method])
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func (f *fakeTransport) Stream(ctx context.Context, method string, p api.Params, result any, notify func(api.Notification)) error {
	return f.Call(ctx, "stream:"+method, p, result, notify)
}

func (f *fakeTransport) Subscribe(_ context.Context, options api.Options, ready func(), notify func(api.Notification)) error {
	f.calls = append(f.calls, call{method: "subscribe", params: api.Params{Options: options}})
	if f.subErr != nil {
		return f.subErr
	}
	ready()
	for _, note := range f.notes {
		notify(note)
	}
	return context.Canceled
}

func noDaemonTransport() *fakeTransport {
	errs := map[string]error{}
	for _, method := range []string{"status.get", "plan.get", "stream:sync.run", "config.get", "config.set"} {
		errs[method] = platform.ErrNoDaemon
	}
	return &fakeTransport{errs: errs, subErr: platform.ErrNoDaemon}
}

// expect fails the test unless ok; one condition per call keeps tests flat.
func expect(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

func kindOf(t *testing.T, err error) model.ErrorKind {
	t.Helper()
	var classified *model.Error
	if !errors.As(err, &classified) {
		t.Fatalf("unclassified error %v", err)
	}
	return classified.Kind
}

func TestStatusFromDaemon(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	transport := &fakeTransport{results: map[string]any{"status.get": workflow.Status{
		StatusSchema: 1, Repository: "/repo", Warnings: []string{"w"},
		LastSync: &state.SyncRecord{At: at, Result: 1, Changed: 2},
		Git:      workflow.GitStatus{Branch: "main", Upstream: "origin/main", Ahead: 1, Behind: 2, Dirty: true},
		Drift:    map[plan.Classification]int{plan.InSync: 5, plan.UpdateNeeded: 4, plan.LocalDrift: 3, plan.Conflict: 2, plan.UnmanagedObstacle: 1},
	}}}
	options := api.Options{Host: "laptop", Mode: "copy"}
	snapshot, err := Client{Transport: transport, Options: options}.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := model.DriftCounts{InSync: 5, UpdateNeeded: 4, LocalDrift: 3, Conflict: 2, UnmanagedObstacle: 1}
	if !snapshot.Daemon || snapshot.Repository != "/repo" || snapshot.Drift != want || snapshot.Git.Behind != 2 || !snapshot.Git.Dirty {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if *snapshot.LastSync != (model.LastSync{At: at, ExitCode: 1, Changed: 2}) || snapshot.ToolTooOld != nil {
		t.Fatalf("last sync=%+v", snapshot.LastSync)
	}
	if transport.calls[0].params.Options != options {
		t.Fatalf("options not forwarded: %+v", transport.calls[0])
	}
}

func TestStatusFallsBackOnlyWithoutDaemon(t *testing.T) {
	t.Parallel()
	tooOld := &manifest.VersionError{Required: "9.0.0", Current: "1.0.0"}
	local := Local{Workflow: func(context.Context) (workflow.Service, error) { return workflow.Service{}, tooOld }}
	snapshot, err := Client{Transport: noDaemonTransport(), Local: local}.Status(t.Context())
	if kindOf(t, err) != model.ErrToolTooOld || snapshot.Daemon || *snapshot.ToolTooOld != (model.ToolTooOld{Required: "9.0.0", Current: "1.0.0"}) {
		t.Fatalf("fallback=%+v err=%v", snapshot, err)
	}
	newer := Local{Workflow: func(context.Context) (workflow.Service, error) {
		return workflow.Service{}, &manifest.SchemaError{Found: 2, Supported: 1}
	}}
	snapshot, err = Client{Transport: noDaemonTransport(), Local: newer}.Status(t.Context())
	if kindOf(t, err) != model.ErrToolTooOld || *snapshot.ToolTooOld != (model.ToolTooOld{Schema: 2, SupportedSchema: 1}) {
		t.Fatalf("schema fallback=%+v err=%v", snapshot, err)
	}
	if _, err = (Client{Transport: noDaemonTransport()}).Status(t.Context()); kindOf(t, err) != model.ErrUnavailable {
		t.Fatalf("no fallback=%v", err)
	}
	called := false
	local.Workflow = func(context.Context) (workflow.Service, error) { called = true; return workflow.Service{}, nil }
	rejected := &fakeTransport{errs: map[string]error{"status.get": &api.Error{Code: api.Unauthorized, Data: api.ErrorData{ExitCode: 2}}}}
	if _, err = (Client{Transport: rejected, Local: local}).Status(t.Context()); kindOf(t, err) != model.ErrUnauthorized || called {
		t.Fatalf("authentication failure fell back locally: %v called=%v", err, called)
	}
	if event := compatibility.FromError(tooOld, ""); event == nil {
		t.Fatal("fixture is not a version error")
	}
}

func TestDriftProjection(t *testing.T) {
	t.Parallel()
	result := api.PlanResult{Warnings: []string{"adapter"}, Plan: plan.Plan{
		Observations: []plan.Observation{{Path: "/a", Classification: plan.InSync}, {Path: "/b", Classification: plan.LocalDrift}, {Path: "/c", Classification: plan.UpdateNeeded}},
		Operations: []plan.Operation{
			{Kind: plan.Backup, Path: "/b", BackupPath: "/b.bak"},
			{Kind: plan.Materialize, Path: "/b", Desired: &plan.Desired{SourcePath: "/repo/b"}},
			{Kind: plan.Skip, Path: "/c", Warning: "unsupported"},
			{Kind: plan.Record, Path: "/a"},
		},
	}}
	snapshot, err := Client{Transport: &fakeTransport{results: map[string]any{"plan.get": result}}}.Drift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DriftItem{
		{Path: "/b", Classification: model.LocalDrift, Actions: []model.Action{{Kind: model.ActionBackup, BackupPath: "/b.bak"}, {Kind: model.ActionMaterialize, SourcePath: "/repo/b"}}},
		{Path: "/c", Classification: model.UpdateNeeded, Actions: []model.Action{{Kind: model.ActionSkip, Warning: "unsupported"}}},
	}
	expect(t, snapshot.Daemon && snapshot.Warnings[0] == "adapter", "snapshot=%+v", snapshot)
	expect(t, len(snapshot.Items) == len(want), "items=%+v", snapshot.Items)
	for i := range want {
		got := snapshot.Items[i]
		expect(t, got.Path == want[i].Path && got.Classification == want[i].Classification, "item %d=%+v", i, got)
		expect(t, slices.Equal(got.Actions, want[i].Actions), "item %d actions=%+v", i, got.Actions)
	}
	local := Local{Workflow: func(context.Context) (workflow.Service, error) { return workflow.Service{}, fs.ErrInvalid }}
	if snapshot, err = (Client{Transport: noDaemonTransport(), Local: local}).Drift(t.Context()); kindOf(t, err) != model.ErrInvalid || snapshot.Daemon {
		t.Fatalf("fallback=%+v %v", snapshot, err)
	}
}

func TestSyncStreamsProgress(t *testing.T) {
	t.Parallel()
	transport := &fakeTransport{
		results: map[string]any{"stream:sync.run": api.SyncResult{ExitCode: 1, Changed: 3}},
		notes: []api.Notification{
			{Method: "sync.progress", Params: api.Event{Phase: "started"}},
			{Method: "sync.started"},
			{Method: "sync.progress", Params: api.Event{Phase: "apply"}},
		},
	}
	var phases []string
	outcome, err := Client{Transport: transport}.Sync(t.Context(), func(phase string) { phases = append(phases, phase) })
	expect(t, err == nil && outcome == (model.SyncOutcome{ExitCode: 1, Changed: 3}), "outcome=%+v err=%v", outcome, err)
	expect(t, strings.Join(phases, ",") == "started,apply", "phases=%v", phases)
	conflict := &fakeTransport{errs: map[string]error{"stream:sync.run": &api.Error{Code: api.OperationFailed, Data: api.ErrorData{ExitCode: 3}}}}
	outcome, err = Client{Transport: conflict}.Sync(t.Context(), func(string) {})
	expect(t, kindOf(t, err) == model.ErrConflict && outcome.ExitCode == 3, "conflict=%+v %v", outcome, err)
	_, err = Client{Transport: noDaemonTransport()}.Sync(t.Context(), func(string) {})
	expect(t, kindOf(t, err) == model.ErrUnavailable, "no fallback=%v", err)
}

func TestSyncFallsBackInProcess(t *testing.T) {
	t.Parallel()
	local := Local{Workflow: func(context.Context) (workflow.Service, error) { return workflow.Service{}, platform.ErrLocked }}
	outcome, err := Client{Transport: noDaemonTransport(), Local: local}.Sync(t.Context(), func(string) {})
	expect(t, kindOf(t, err) == model.ErrBusy && outcome.ExitCode == 1, "local failure=%+v %v", outcome, err)
	var phases []string
	local.Workflow = func(context.Context) (workflow.Service, error) { return workflow.Service{Store: &state.Store{}}, nil }
	outcome, err = Client{Transport: noDaemonTransport(), Local: local}.Sync(t.Context(), func(phase string) { phases = append(phases, phase) })
	// An unconfigured service fails its own preconditions after reporting the phases.
	expect(t, err != nil && outcome.ExitCode > 0, "local sync=%+v err=%v", outcome, err)
	expect(t, strings.Join(phases, ",") == "started,finished", "phases=%v", phases)
}

func TestSubscribeEvents(t *testing.T) {
	t.Parallel()
	transport := &fakeTransport{notes: []api.Notification{{Method: "sync.finished", Params: api.Event{ExitCode: 1, Changed: 2}}, {Method: "update.available"}}}
	var events []model.Event
	err := Client{Transport: transport}.Subscribe(t.Context(), func(e model.Event) { events = append(events, e) })
	want := []model.Event{{Kind: model.EventConnected}, {Kind: model.EventSyncFinished, ExitCode: 1, Changed: 2}, {Kind: model.EventUpdateAvailable}}
	if kindOf(t, err) != model.ErrCanceled || len(events) != len(want) {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event %d=%+v", i, events[i])
		}
	}
	events = nil
	if err = (Client{Transport: noDaemonTransport()}).Subscribe(t.Context(), func(e model.Event) { events = append(events, e) }); kindOf(t, err) != model.ErrUnavailable || events != nil {
		t.Fatalf("no daemon=%v events=%v", err, events)
	}
}

func TestClassification(t *testing.T) {
	t.Parallel()
	remote := func(code int, exit int) error {
		return &api.Error{Code: code, Data: api.ErrorData{ExitCode: apiCode(exit)}}
	}
	for _, tc := range []struct {
		err  error
		kind model.ErrorKind
		exit int
	}{
		{context.Canceled, model.ErrCanceled, 1},
		{platform.ErrNoDaemon, model.ErrUnavailable, 1},
		{platform.ErrLocked, model.ErrBusy, 1},
		{fs.ErrInvalid, model.ErrInvalid, 2},
		{&manifest.VersionError{}, model.ErrToolTooOld, 4},
		{&manifest.SchemaError{Found: 2, Supported: 1}, model.ErrToolTooOld, 4},
		{&platform.UnsupportedOSError{}, model.ErrUnsupportedOS, 5},
		{errors.New("other"), model.ErrOperation, 1},
		{remote(api.Busy, 1), model.ErrBusy, 1},
		{remote(api.Unauthorized, 2), model.ErrUnauthorized, 2},
		{remote(api.UnsupportedVersion, 2), model.ErrIncompatible, 2},
		{remote(api.MethodNotFound, 2), model.ErrIncompatible, 2},
		{remote(api.OperationFailed, 2), model.ErrInvalid, 2},
		{remote(api.OperationFailed, 3), model.ErrConflict, 3},
		{remote(api.OperationFailed, 4), model.ErrToolTooOld, 4},
		{remote(api.OperationFailed, 5), model.ErrUnsupportedOS, 5},
		{remote(api.InternalError, 1), model.ErrOperation, 1},
	} {
		var classified *model.Error
		if !errors.As(classify(tc.err), &classified) || classified.Kind != tc.kind || classified.ExitCode != tc.exit || !errors.Is(classified, tc.err) {
			t.Errorf("%v: %+v", tc.err, classified)
		}
	}
	if classify(nil) != nil {
		t.Fatal("nil classified")
	}
}

func TestSettingsEntriesMapSources(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "KITBAG_UI_THEME": "dark"}))
	transport := &fakeTransport{results: map[string]any{"config.get": api.ConfigResult{Config: "schema = 1\n[repo]\nurl = 'local'\n[ui]\ncolor = 'never'\n"}}}
	values, err := Client{Transport: transport, Env: host, Flags: map[string]string{"mode": "copy"}}.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.SettingValue{}
	for _, value := range values {
		got[value.Key] = value
	}
	for key, want := range map[string]model.SettingValue{
		"ui.color":    {Key: "ui.color", Value: "never", Source: model.SourceFile},
		"ui.theme":    {Key: "ui.theme", Value: "dark", Source: model.SourceEnvironment, Name: "KITBAG_UI_THEME"},
		"sync.mode":   {Key: "sync.mode", Value: "copy", Source: model.SourceFlag, Name: "--mode"},
		"git.backend": {Key: "git.backend", Value: "auto", Source: model.SourceDefault},
	} {
		if got[key] != want {
			t.Errorf("%s=%+v want %+v", key, got[key], want)
		}
	}
	if _, err = (Client{Transport: transport}).Settings(t.Context()); kindOf(t, err) != model.ErrInvalid {
		t.Fatalf("missing environment=%v", err)
	}
	if _, err = (Client{Transport: noDaemonTransport()}).Settings(t.Context()); kindOf(t, err) != model.ErrUnavailable {
		t.Fatalf("no fallback=%v", err)
	}
}

func TestSettingChangesThroughTheAPI(t *testing.T) {
	t.Parallel()
	transport := &fakeTransport{results: map[string]any{"config.get": api.ConfigResult{Config: "schema = 1\n[repo]\nurl = 'local'\n"}}}
	client := Client{Transport: transport, Options: api.Options{Config: "/custom.toml"}}
	if err := client.SetSetting(t.Context(), "ui.theme", "light"); err != nil {
		t.Fatal(err)
	}
	set := transport.calls[1]
	if set.method != "config.set" || !strings.Contains(set.params.Config, "theme = 'light'") || set.params.Options.Config != "/custom.toml" {
		t.Fatalf("config.set=%+v", set)
	}
	if err := client.UnsetSetting(t.Context(), "repo.url"); kindOf(t, err) != model.ErrInvalid || len(transport.calls) != 3 {
		t.Fatalf("removing a required key reached config.set: %v calls=%d", err, len(transport.calls))
	}
	transport.results["config.get"] = api.ConfigResult{Config: "# comment\nschema = 1\n[repo]\nurl = 'local'\n"}
	if err := client.SetSetting(t.Context(), "ui.theme", "dark"); kindOf(t, err) != model.ErrInvalid || len(transport.calls) != 4 {
		t.Fatalf("comment-bearing file rewritten: %v", err)
	}
	transport.errs = map[string]error{"config.get": &api.Error{Code: api.Busy, Data: api.ErrorData{ExitCode: 1}}}
	if err := client.SetSetting(t.Context(), "ui.theme", "dark"); kindOf(t, err) != model.ErrBusy {
		t.Fatalf("busy=%v", err)
	}
}

func TestSettingsLocalFallback(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	service := settings.Service{Host: host, Filename: "/home/test/.config/kitbag/config.toml", Flags: map[string]string{}}
	client := Client{Transport: noDaemonTransport(), Local: Local{Settings: func() (settings.Service, error) { return service, nil }}}
	expect(t, client.SetSetting(t.Context(), "repo.url", "local") == nil, "set repo.url")
	expect(t, client.SetSetting(t.Context(), "ui.theme", "light") == nil, "set ui.theme")
	expect(t, client.UnsetSetting(t.Context(), "ui.theme") == nil, "unset ui.theme")
	values, err := client.Settings(t.Context())
	expect(t, err == nil, "list=%v", err)
	got := map[string]model.SettingValue{}
	for _, value := range values {
		got[value.Key] = value
	}
	expect(t, got["repo.url"] == model.SettingValue{Key: "repo.url", Value: "local", Source: model.SourceFile}, "repo.url=%+v", got["repo.url"])
	expect(t, got["ui.theme"].Source == model.SourceDefault, "ui.theme=%+v", got["ui.theme"])
	broken := Client{Transport: noDaemonTransport(), Local: Local{Settings: func() (settings.Service, error) { return settings.Service{}, fs.ErrInvalid }}}
	_, err = broken.Settings(t.Context())
	expect(t, kindOf(t, err) == model.ErrInvalid, "broken list=%v", err)
	expect(t, kindOf(t, broken.SetSetting(t.Context(), "ui.theme", "dark")) == model.ErrInvalid, "broken set")
}

func apiCode(exit int) apply.Code { return apply.Code(exit) }
