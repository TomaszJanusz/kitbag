package api

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestEventBackPressure(t *testing.T) {
	t.Parallel()
	broker := NewEvents()
	slow, unsubscribe := broker.subscribe()
	defer unsubscribe()
	if cap(slow) != EventBuffer {
		t.Fatalf("queue capacity=%d, want %d", cap(slow), EventBuffer)
	}
	done := make(chan struct{})
	go func() {
		for range EventBuffer * 4 {
			broker.Publish(Event{Kind: "drift.detected"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber blocked publisher")
	}
	if len(slow) != EventBuffer {
		t.Fatalf("queue length=%d", len(slow))
	}
	assertDroppedOverflow(t, slow)
	fast, stop := broker.subscribe()
	defer stop()
	broker.Publish(Event{Kind: "update.available"})
	if event := <-fast; event.Kind != "update.available" {
		t.Fatalf("fast event=%+v", event)
	}
}

func TestTokenLifetime(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	token, err := EnsureToken(host)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EnsureToken(host)
	if err != nil || again != token || len(token) != tokenLength {
		t.Fatalf("token was replaced or invalid: %v", err)
	}
	if !authorized(token, token) || authorized(token, "") || authorized(token, "wrong") {
		t.Fatal("authorization decision incorrect")
	}
}

func TestInsecureTokenRejected(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	if _, err := EnsureToken(host); err != nil {
		t.Fatal(err)
	}
	path, err := tokenPath(host)
	if err != nil {
		t.Fatal(err)
	}
	if err = host.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = EnsureToken(host); err == nil {
		t.Fatal("accepted insecure token")
	}
	if err = host.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedTokenPreserved(t *testing.T) {
	t.Parallel()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}))
	if _, err := EnsureToken(host); err != nil {
		t.Fatal(err)
	}
	path, err := tokenPath(host)
	if err != nil {
		t.Fatal(err)
	}
	if err = host.WriteAtomic(path, []byte("secret-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = EnsureToken(host); err == nil {
		t.Fatal("replaced malformed token")
	}
}

func TestNotificationQueueBound(t *testing.T) {
	t.Parallel()
	c := &connection{out: make(chan outgoing, EventBuffer)}
	for range EventBuffer * 4 {
		c.notify(Event{Kind: "sync.progress"})
	}
	if len(c.out) != EventBuffer || cap(c.out) != EventBuffer {
		t.Fatal("notification queue is not bounded")
	}
}

func TestServeRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if err := Serve(context.Background(), nil, Handlers{}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid handlers: %v", err)
	}
}

func assertDroppedOverflow(t *testing.T, queue chan Event) {
	t.Helper()
	for range EventBuffer {
		<-queue
	}
	const overflowWindow = 10 * time.Millisecond
	select {
	case <-queue:
		t.Fatal("overflow retained events outside the bounded queue")
	case <-time.After(overflowWindow):
	}
}

func TestEventsCarryDaemonVersion(t *testing.T) {
	t.Parallel()
	broker := NewEvents("0.1.3")
	queue, unsubscribe := broker.subscribe()
	defer unsubscribe()
	for _, kind := range []string{"sync.started", "sync.finished", "drift.detected", "update.available", "poll.finished"} {
		broker.Publish(Event{Kind: kind})
		event := <-queue
		if event.DaemonVersion != "0.1.3" || event.APIVersion != Version {
			t.Fatal(event)
		}
	}
}
