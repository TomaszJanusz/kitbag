package tui

import (
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestCheckFailureWarning(t *testing.T) {
	t.Parallel()
	client := newFake()
	client.status.LastCheck = &model.LastCheck{At: clock().Add(-4 * time.Minute), NextAt: clock().Add(time.Minute), Outcome: "failed", Category: "operation", Interval: time.Minute}
	s := start(t, client)
	s.waitFor("daemon is not checking", "failed: The operation failed", "Next check")
	golden(t, "status-check-failed", s.final())
}

func TestCheckEventRefreshesStatus(t *testing.T) {
	t.Parallel()
	client := newFake()
	s := start(t, client)
	s.waitFor("No check recorded", "Daemon, live updates")
	client.mu.Lock()
	client.status.LastCheck = &model.LastCheck{At: clock(), NextAt: clock().Add(time.Minute), Outcome: "ok", Interval: time.Minute}
	client.mu.Unlock()
	client.events <- model.Event{Kind: model.EventPollFinished}
	s.waitFor("just now), ok", "Next check")
	s.final()
}
