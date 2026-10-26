package apiclient

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// daemon serves the production handlers over the fake socket. Workflow calls
// fail (no repository) while config.get/config.set use the real settings service.
func daemon(t *testing.T, host *fake.Host) *api.Events {
	t.Helper()
	token, err := api.EnsureToken(host)
	if err != nil {
		t.Fatal(err)
	}
	path, err := api.SocketPath(host)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := host.ListenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	events := api.NewEvents()
	factory := func(context.Context, api.Options) (workflow.Service, error) { return workflow.Service{}, fs.ErrInvalid }
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- api.Serve(ctx, listener, api.Handlers{Host: host, Token: token, Events: events, Service: factory})
	}()
	t.Cleanup(func() {
		cancel()
		if serveErr := <-done; serveErr != nil {
			t.Error(serveErr)
		}
	})
	return events
}

// harness drives an App synchronously; Subscribe runs in the background and
// reports through results, like a renderer's worker.
type harness struct {
	ctx     context.Context
	app     *model.App
	results chan model.Msg
	runner  model.Runner
	pending []model.Cmd
}

func (h *harness) drain() {
	for len(h.pending) > 0 {
		cmd := h.pending[0]
		h.pending = h.pending[1:]
		if _, ok := cmd.(model.Subscribe); ok {
			go h.runner.Run(h.ctx, cmd, func(msg model.Msg) { h.results <- msg })
			continue
		}
		h.runner.Run(h.ctx, cmd, func(msg model.Msg) { h.pending = append(h.pending, h.app.Update(msg)...) })
	}
}

func (h *harness) dispatch(msg model.Msg) {
	h.pending = append(h.pending, h.app.Update(msg)...)
	h.drain()
}

func TestModelsOverTheLocalAPI(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "KITBAG_CONFIG": "/home/test/config.toml"}))
	events := daemon(t, host)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h := &harness{ctx: ctx, app: model.NewApp(model.DefaultLogCapacity, timefmt.Clock{}), results: make(chan model.Msg, 16)}
	h.runner = model.Runner{Client: Client{Transport: client.Client{Host: host}, Env: host}, Now: time.Now}
	h.pending = h.app.Init()
	h.drain()
	// The daemon cannot build a workflow here: its sanitized usage error is classified.
	view := h.app.Status.View()
	if view.Phase != model.Failed || view.Headline.Key != text.ErrorInvalid {
		t.Fatalf("status=%+v", view)
	}
	h.dispatch(model.EditSetting{Key: "repo.url", Value: "https://example.invalid/config.git"})
	h.dispatch(model.SaveSetting{Key: "repo.url"})
	if stored, err := host.ReadFile("/home/test/config.toml"); err != nil || len(stored) == 0 {
		t.Fatalf("config.set did not persist: %v", err)
	}
	saved := h.app.Settings.View().Fields[2]
	if saved.Path != "repo.url" || saved.Source.Key != text.SourceFile || saved.Problem != nil {
		t.Fatalf("saved field=%+v", saved)
	}
	waitFor(t, h, eventOf(model.EventConnected))
	events.Publish(api.Event{Kind: "update.available"})
	waitFor(t, h, eventOf(model.EventUpdateAvailable))
	if notices := h.app.Status.View().Notices; len(notices) == 0 || notices[0].Key != text.StatusUpdate {
		t.Fatalf("notices=%+v", notices)
	}
	cancel()
	waitFor(t, h, func(m model.Msg) bool { _, ok := m.(model.SubscriptionEnded); return ok })
}

func eventOf(want model.EventKind) func(model.Msg) bool {
	return func(m model.Msg) bool {
		received, ok := m.(model.EventReceived)
		return ok && received.Event.Kind == want
	}
}

func waitFor(t *testing.T, h *harness, match func(model.Msg) bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg := <-h.results:
			h.app.Update(msg)
			if match(msg) {
				return
			}
		case <-timeout:
			t.Fatal("message not received")
		}
	}
}
