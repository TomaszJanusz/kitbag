// Package daemon schedules serialized remote synchronization and read-only drift
// evaluation over injected clocks, detectors and event publishers.
package daemon

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

const (
	// MinPoll prevents accidental busy polling, including in integration tests.
	MinPoll        = time.Second
	maxPoll        = 24 * time.Hour
	minDebounce    = 100 * time.Millisecond
	maxBackoff     = 15 * time.Minute
	rescanInterval = time.Minute
	updateInterval = 24 * time.Hour
)

// Timer is a resettable timer; implementations use nonblocking Stop and Reset.
type Timer interface {
	C() <-chan time.Time
	Reset(time.Duration)
	Stop()
}

// Clock constructs timers. Tests advance a virtual clock without sleeping.
type Clock interface{ NewTimer(time.Duration) Timer }

// Options are fixed for a daemon lifetime; restart after changing settings.
type Options struct {
	Poll, Debounce time.Duration
	UpdateCheck    bool
}

// Dependencies never run concurrently with one another. Watch hints may be
// dropped/coalesced: periodic scans remain authoritative. Publish must not block.
type Dependencies struct {
	Clock   Clock
	Probe   func(context.Context) (string, error)
	Sync    func(context.Context) (string, error)
	Scan    func(context.Context) error
	Update  func(context.Context) (bool, error)
	Publish func(string)
	Log     func(string, error)
	Jitter  func(time.Duration) time.Duration
	Watch   <-chan struct{}
}

// Run performs an initial synchronization, probes with bounded jitter/backoff,
// coalesces watcher hints, and scans periodically even if watching fails. It
// finishes an active call before returning on cancellation; apply journals own
// crash recovery. No goroutine is started for a sync, so runs cannot overlap.
func Run(ctx context.Context, options Options, deps Dependencies) error {
	if err := validate(options, deps); err != nil {
		return err
	}
	loop := scheduler{options: options, deps: deps, delay: options.Poll, watch: deps.Watch}
	loop.poll = deps.Clock.NewTimer(MinPoll)
	loop.scan = deps.Clock.NewTimer(rescanInterval)
	loop.update = deps.Clock.NewTimer(updateInterval)
	loop.debounce = deps.Clock.NewTimer(options.Debounce)
	loop.debounce.Stop()
	defer loop.poll.Stop()
	defer loop.scan.Stop()
	defer loop.update.Stop()
	defer loop.debounce.Stop()
	var err error
	loop.head, err = synchronize(ctx, deps)
	deps.Log("sync", err)
	loop.checkUpdate(ctx)
	loop.poll.Reset(jitter(deps, loop.delay))
	return loop.run(ctx)
}

type scheduler struct {
	deps                         Dependencies
	poll, scan, update, debounce Timer
	watch                        <-chan struct{}
	debounceC                    <-chan time.Time
	head                         string
	options                      Options
	delay                        time.Duration
}

func (s *scheduler) run(ctx context.Context) error {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil
		case _, open := <-s.watch:
			s.hint(open)
		case <-s.debounceC:
			s.debounceC = nil
			s.deps.Log("scan", s.deps.Scan(ctx))
		case <-s.scan.C():
			s.deps.Log("scan", s.deps.Scan(ctx))
			s.scan.Reset(rescanInterval)
		case <-s.update.C():
			s.checkUpdate(ctx)
			s.update.Reset(updateInterval)
		case <-s.poll.C():
			s.pollOnce(ctx)
		}
	}
	return nil
}

func (s *scheduler) hint(open bool) {
	if !open {
		s.watch = nil
		return
	}
	if s.debounceC == nil {
		s.debounce.Reset(s.options.Debounce)
		s.debounceC = s.debounce.C()
	}
}

func (s *scheduler) checkUpdate(ctx context.Context) {
	if s.options.UpdateCheck {
		checkUpdate(ctx, s.deps)
	}
}

func (s *scheduler) pollOnce(ctx context.Context) {
	const multiplier = 2
	var err error
	s.head, err = pollOnce(ctx, s.deps, s.head)
	s.deps.Log("poll", err)
	if err != nil {
		s.delay = min(max(s.delay, s.options.Poll)*multiplier, max(maxBackoff, s.options.Poll))
	} else {
		s.delay = s.options.Poll
	}
	s.poll.Reset(jitter(s.deps, s.delay))
}

func validate(o Options, d Dependencies) error {
	if o.Poll < MinPoll || o.Poll > maxPoll || o.Debounce < minDebounce || o.Debounce > time.Minute {
		return fs.ErrInvalid
	}
	required := []bool{d.Clock != nil, d.Probe != nil, d.Sync != nil, d.Scan != nil, d.Publish != nil, d.Log != nil, d.Jitter != nil}
	for _, present := range required {
		if !present {
			return fs.ErrInvalid
		}
	}
	if o.UpdateCheck && d.Update == nil {
		return fs.ErrInvalid
	}
	return nil
}

func jitter(d Dependencies, delay time.Duration) time.Duration {
	// Even a misbehaving jitter source cannot schedule zero or an unbounded delay.
	const tenth = 10
	return max(MinPoll, min(delay, max(delay-delay/tenth, d.Jitter(delay))))
}

func pollOnce(ctx context.Context, d Dependencies, previous string) (string, error) {
	if previous == "" {
		return synchronize(ctx, d)
	}
	remote, err := d.Probe(ctx)
	if err != nil {
		return previous, err
	}
	if remote == "" {
		return previous, fs.ErrInvalid
	}
	if remote == previous {
		return previous, nil
	}
	local, err := synchronize(ctx, d)
	if err != nil {
		return previous, err
	}
	if local != remote {
		return previous, errors.New("upstream not applied; working copy may have local changes")
	}
	return remote, nil
}

func synchronize(ctx context.Context, d Dependencies) (string, error) {
	d.Publish("sync.started")
	head, err := d.Sync(ctx)
	d.Publish("sync.finished")
	return head, err
}

func checkUpdate(ctx context.Context, d Dependencies) {
	available, err := d.Update(ctx)
	d.Log("update", err)
	if err == nil && available {
		d.Publish("update.available")
	}
}
