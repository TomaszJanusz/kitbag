package tui

import (
	"slices"
	"sync"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// queue is the unbounded FIFO of the serial worker. Commands run one at a
// time: the in-process services and the daemon's per-request services share
// the state lock, so concurrent loads would fail as busy.
type queue struct {
	wake    chan struct{}
	pending []model.Cmd
	mu      sync.Mutex
}

func newQueue() *queue { return &queue{wake: make(chan struct{}, 1)} }

// push appends cmd; a queued load of the same snapshot is superseded by the
// newer request, whose result the model applies instead.
func (q *queue) push(cmd model.Cmd) {
	q.mu.Lock()
	q.pending = slices.DeleteFunc(q.pending, func(queued model.Cmd) bool { return supersedes(cmd, queued) })
	q.pending = append(q.pending, cmd)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *queue) pop() (model.Cmd, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil, false
	}
	cmd := q.pending[0]
	q.pending = q.pending[1:]
	return cmd, true
}

func supersedes(next, queued model.Cmd) bool {
	switch next.(type) {
	case model.LoadStatus:
		_, same := queued.(model.LoadStatus)
		return same
	case model.LoadDrift:
		_, same := queued.(model.LoadDrift)
		return same
	case model.LoadSettings:
		_, same := queued.(model.LoadSettings)
		return same
	case model.LoadAgents:
		_, same := queued.(model.LoadAgents)
		return same
	default:
		return false
	}
}

// start queues commands for the serial worker. A subscription runs on its own
// worker because it lasts until the stream ends.
func (m *Model) start(cmds []model.Cmd) {
	for _, cmd := range cmds {
		if _, long := cmd.(model.Subscribe); long {
			m.workers.Add(1)
			go func() {
				defer m.workers.Done()
				m.run(cmd)
			}()
			continue
		}
		m.queue.push(cmd)
	}
}

// serve runs queued commands one at a time until the renderer shuts down.
func (m *Model) serve() {
	defer m.workers.Done()
	for {
		cmd, ok := m.queue.pop()
		switch {
		case m.ctx.Err() != nil:
			return
		case ok:
			m.run(cmd)
			continue
		}
		select {
		case <-m.queue.wake:
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Model) run(cmd model.Cmd) {
	defer func() {
		if value := recover(); value != nil {
			m.emit(panicMsg{value: value})
		}
	}()
	m.runner.Run(m.ctx, cmd, func(msg model.Msg) { m.emit(msg) })
}

// emit delivers to the program loop unless the renderer is shutting down.
func (m *Model) emit(msg any) {
	select {
	case m.results <- msg:
	case <-m.ctx.Done():
	}
}
