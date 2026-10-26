// Package daemon schedules serialized remote synchronization and read-only drift
// evaluation over injected clocks, detectors and event publishers.
package daemon

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
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

// ErrReplaced reports that Replaced confirmed a different executable on disk.
// Run returns it between operations; the caller exits so that its service
// manager starts the new version.
var ErrReplaced = errors.New("executable replaced on disk")

// Timer is a resettable timer; implementations use nonblocking Stop and Reset.
type Timer interface {
	C() <-chan time.Time
	Reset(time.Duration)
	Stop()
}

// Clock constructs timers. Tests advance a virtual clock without sleeping.
type Clock interface{ NewTimer(time.Duration) Timer }

// Options are fixed for a daemon lifetime; restart after changing settings.
// A nonzero ExecutableCheck asks Replaced at that interval.
type Options struct {
	Poll, Debounce, ExecutableCheck time.Duration
	UpdateCheck                     bool
}

// Dependencies never run concurrently with one another. Watch hints may be
// dropped/coalesced: periodic scans remain authoritative. Publish must not block.
// Optional Trim returns idle memory after timer-driven work (initial sync, poll,
// periodic scan, update check), so it runs at most a few times per poll interval
// and never once per watch hint. Optional Now/Checked record completed polls;
// Checked receives only safe categories and hex hashes and must not block long.
type Dependencies struct {
	Now      func() time.Time
	Checked  func(state.CheckRecord)
	Clock    Clock
	Probe    func(context.Context) (string, error)
	Sync     func(context.Context) (string, error)
	Scan     func(context.Context) error
	Update   func(context.Context) (bool, error)
	Publish  func(string)
	Log      func(string, error)
	Jitter   func(time.Duration) time.Duration
	Watch    <-chan struct{}
	Trim     func()
	Replaced func(context.Context) (bool, error)
}

// Run performs an initial synchronization, probes with bounded jitter/backoff,
// coalesces watcher hints, and scans periodically even if watching fails. It
// finishes an active call before returning on cancellation; apply journals own
// crash recovery. No goroutine is started for a sync, so runs cannot overlap.
// A confirmed replaced executable returns ErrReplaced after the active call.
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
	if options.ExecutableCheck > 0 {
		loop.replace = deps.Clock.NewTimer(options.ExecutableCheck)
		defer loop.replace.Stop()
		loop.replaceC = loop.replace.C()
	}
	var err error
	loop.head, err = synchronize(ctx, deps)
	deps.Log("sync", err)
	loop.checkUpdate(ctx)
	loop.trim()
	loop.schedule(err, loop.head)
	return loop.run(ctx)
}

type scheduler struct {
	deps                         Dependencies
	poll, scan, update, debounce Timer
	replace                      Timer
	watch                        <-chan struct{}
	debounceC, replaceC          <-chan time.Time
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
			s.trim()
			s.scan.Reset(rescanInterval)
		case <-s.update.C():
			s.checkUpdate(ctx)
			s.trim()
			s.update.Reset(updateInterval)
		case <-s.poll.C():
			s.pollOnce(ctx)
		case <-s.replaceC:
			if s.replaced(ctx) {
				return ErrReplaced
			}
		}
	}
	return nil
}

// replaced logs only failures and the restart decision, so a healthy check
// adds no log record per interval.
func (s *scheduler) replaced(ctx context.Context) bool {
	replaced, err := s.deps.Replaced(ctx)
	if err != nil {
		s.deps.Log("executable", err)
	}
	if replaced {
		s.deps.Log("restart", nil)
		return true
	}
	s.replace.Reset(s.options.ExecutableCheck)
	return false
}

func (s *scheduler) trim() {
	if s.deps.Trim != nil {
		s.deps.Trim()
	}
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
	seen := ""
	deps := s.deps
	deps.Probe = func(ctx context.Context) (string, error) {
		head, failure := s.deps.Probe(ctx)
		seen = head
		return head, failure
	}
	s.head, err = pollOnce(ctx, deps, s.head)
	if seen == "" && err == nil {
		seen = s.head
	}
	s.deps.Log("poll", err)
	s.trim()
	if err != nil {
		s.delay = min(max(s.delay, s.options.Poll)*multiplier, max(maxBackoff, s.options.Poll))
	} else {
		s.delay = s.options.Poll
	}
	s.schedule(err, seen)
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
	return validateExecutableCheck(o, d)
}

func validateExecutableCheck(o Options, d Dependencies) error {
	if o.ExecutableCheck != 0 && (o.ExecutableCheck < MinPoll || o.ExecutableCheck > maxPoll || d.Replaced == nil) {
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

// schedule publishes only fixed categories and validated short hashes.
func (s *scheduler) schedule(failure error, head string) {
	delay := jitter(s.deps, s.delay)
	if s.deps.Checked != nil && s.deps.Now != nil {
		now := s.deps.Now().UTC()
		r := state.CheckRecord{SchemaVersion: 1, At: now, NextAt: now.Add(delay), IntervalSeconds: float64(s.options.Poll) / float64(time.Second), Outcome: "ok", Head: shortHead(head)}
		if failure != nil {
			r.Outcome, r.Category = "failed", category(failure)
		}
		s.deps.Checked(r)
	}
	s.poll.Reset(delay)
}

func shortHead(head string) string {
	const length = 12
	for _, c := range head {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ""
		}
	}
	return head[:min(len(head), length)]
}

func category(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, platform.ErrLocked):
		return "busy"
	case errors.Is(err, fs.ErrInvalid):
		return "invalid"
	}
	switch apply.ErrorCode(err) {
	case apply.Usage:
		return "invalid"
	case apply.Conflict:
		return "conflict"
	case apply.ToolTooOld:
		return "tool_too_old"
	case apply.UnsupportedOS:
		return "unsupported_os"
	case apply.OK, apply.Warnings:
		return "operation"
	default:
		return "operation"
	}
}
