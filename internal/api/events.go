package api

import (
	"encoding/json"
	"sync"
)

// Event is a content-free lifecycle notification. Snapshots remain authoritative.
type Event struct {
	DaemonVersion string          `json:"daemon_version,omitempty"`
	APIVersion    string          `json:"api_version"`
	Kind          string          `json:"kind"`
	Phase         string          `json:"phase,omitempty"`
	RequestID     json.RawMessage `json:"request_id,omitempty"`
	ExitCode      int             `json:"exit_code"`
	Changed       int             `json:"changed"`
}

// Events broadcasts lifecycle hints through fixed-size, drop-newest queues.
// A slow subscriber never blocks a publisher or another subscriber.
type Events struct {
	subscribers map[chan Event]bool
	version     string
	mu          sync.Mutex
}

// NewEvents constructs a broker with no background workers.
func NewEvents(version ...string) *Events {
	e := &Events{subscribers: make(map[chan Event]bool)}
	if len(version) > 0 {
		e.version = version[0]
	}
	return e
}

// Publish sends a supported lifecycle hint without waiting for subscribers.
// The daemon uses this for drift.detected and update.available after detection.
func (e *Events) Publish(event Event) {
	event.APIVersion = Version
	if event.DaemonVersion == "" {
		event.DaemonVersion = e.version
	}
	if !eventKind(event.Kind) {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for queue := range e.subscribers {
		select {
		case queue <- event:
		default:
		}
	}
}

func eventKind(kind string) bool {
	switch kind {
	case "poll.finished", "sync.started", "sync.finished", "drift.detected", "update.available":
		return true
	default:
		return false
	}
}

func (e *Events) subscribe() (chan Event, func()) {
	queue := make(chan Event, EventBuffer)
	e.mu.Lock()
	e.subscribers[queue] = true
	e.mu.Unlock()
	return queue, func() { e.mu.Lock(); delete(e.subscribers, queue); e.mu.Unlock() }
}
