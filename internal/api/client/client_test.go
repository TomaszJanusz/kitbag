package client

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

const waitLimit = 5 * time.Second

func newHost(t *testing.T) *fake.Host {
	t.Helper()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	if _, err := api.EnsureToken(host); err != nil {
		t.Fatal(err)
	}
	return host
}

// serve runs the production server over the fake socket; token overrides the
// stored token to model a rotated or foreign credential.
func serve(t *testing.T, host *fake.Host, token string) *api.Events {
	t.Helper()
	if token == "" {
		stored, err := api.LoadToken(host)
		if err != nil {
			t.Fatal(err)
		}
		token = stored
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
		done <- api.Serve(ctx, listener, api.Handlers{Host: host, Token: token, Events: events, ToolVersion: "1.2.3", Service: factory})
	}()
	t.Cleanup(func() {
		cancel()
		if serveErr := <-done; serveErr != nil {
			t.Error(serveErr)
		}
	})
	return events
}

func TestSubscribeForwardsHintsAcrossKeepAlives(t *testing.T) {
	t.Parallel()
	host := newHost(t)
	events := serve(t, host, "")
	ctx, cancel := context.WithCancel(t.Context())
	ready, received, result := make(chan struct{}), make(chan api.Notification, 4), make(chan error, 1)
	subscriber := Client{Host: host, KeepAlive: time.Millisecond}
	go func() {
		result <- subscriber.Subscribe(ctx, api.Options{}, func() { close(ready) }, func(n api.Notification) { received <- n })
	}()
	wait(t, ready)
	// Several keepalive results arrive before the hint; none may end the stream.
	time.Sleep(20 * time.Millisecond)
	events.Publish(api.Event{Kind: "update.available"})
	select {
	case n := <-received:
		if n.Method != "update.available" || n.Params.Kind != "update.available" || n.Params.APIVersion != api.Version {
			t.Fatalf("notification=%+v", n)
		}
	case err := <-result:
		t.Fatalf("subscription ended early: %v", err)
	case <-time.After(waitLimit):
		t.Fatal("hint not delivered")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestSubscribeFailures(t *testing.T) {
	t.Parallel()
	absent := newHost(t)
	called := false
	err := Client{Host: absent}.Subscribe(t.Context(), api.Options{}, func() { called = true }, nil)
	if !errors.Is(err, platform.ErrNoDaemon) || called {
		t.Fatalf("no daemon=%v ready=%v", err, called)
	}
	host := newHost(t)
	serve(t, host, strings.Repeat("0", 64))
	err = Client{Host: host}.Subscribe(t.Context(), api.Options{}, func() { called = true }, nil)
	var problem *api.Error
	if !errors.As(err, &problem) || problem.Code != api.Unauthorized || called {
		t.Fatalf("foreign token=%v ready=%v", err, called)
	}
}

func TestStreamAndCallShareTheEnvelope(t *testing.T) {
	t.Parallel()
	host := newHost(t)
	serve(t, host, "")
	var streamed, called api.VersionInfo
	if err := (Client{Host: host}).Stream(t.Context(), "version.get", api.Params{}, &streamed, nil); err != nil {
		t.Fatal(err)
	}
	if err := (Client{Host: host, APIVersion: "1.7"}).Call(t.Context(), "version.get", api.Params{}, &called, nil); err != nil {
		t.Fatal(err)
	}
	if streamed.ToolVersion != "1.2.3" || called.MaxVersion != api.Version {
		t.Fatalf("stream=%+v call=%+v", streamed, called)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (Client{Host: host}).Stream(ctx, "version.get", api.Params{}, &streamed, nil); err == nil {
		t.Fatal("canceled stream succeeded")
	}
}

func wait(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(waitLimit):
		t.Fatal("subscription not confirmed")
	}
}
