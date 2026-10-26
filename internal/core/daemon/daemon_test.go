package daemon

import (
	"context"
	"errors"
	"io/fs"
	"sync/atomic"
	"testing"
	"time"
)

type manualTimer struct {
	tick    chan time.Time
	resets  chan time.Duration
	stopped atomic.Bool
}

func (m *manualTimer) C() <-chan time.Time   { return m.tick }
func (m *manualTimer) Reset(d time.Duration) { m.resets <- d }
func (m *manualTimer) Stop()                 { m.stopped.Store(true) }

type manualClock struct{ timers chan *manualTimer }

func (c manualClock) NewTimer(time.Duration) Timer {
	timer := &manualTimer{tick: make(chan time.Time, 8), resets: make(chan time.Duration, 8)}
	c.timers <- timer
	return timer
}

type running struct {
	poll, scan, update, debounce *manualTimer
	cancel                       context.CancelFunc
	done                         chan error
	hints                        chan struct{}
}

func start(t *testing.T, options Options, deps Dependencies) running {
	t.Helper()
	clock := manualClock{timers: make(chan *manualTimer, 4)}
	hints := make(chan struct{}, 8)
	deps.Clock = clock
	deps.Watch = hints
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, deps) }()
	r := running{poll: receive(t, clock.timers), scan: receive(t, clock.timers), update: receive(t, clock.timers), debounce: receive(t, clock.timers), cancel: cancel, done: done, hints: hints}
	t.Cleanup(func() {
		cancel()
		if err := receive(t, done); err != nil {
			t.Error(err)
		}
		for _, timer := range []*manualTimer{r.poll, r.scan, r.update, r.debounce} {
			if !timer.stopped.Load() {
				t.Error("timer leaked")
			}
		}
	})
	return r
}

func receive[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(time.Second):
		t.Fatal("scheduler did not make progress")
		var zero T
		return zero
	}
}

func defaults() (Options, Dependencies) {
	return Options{Poll: time.Minute, Debounce: time.Second}, Dependencies{
		Probe:   func(context.Context) (string, error) { return "head", nil },
		Sync:    func(context.Context) (string, error) { return "head", nil },
		Scan:    func(context.Context) error { return nil },
		Publish: func(string) {}, Log: func(string, error) {}, Jitter: func(d time.Duration) time.Duration { return d },
	}
}

func TestPollBackoffAndRetry(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	var attempts, syncs atomic.Int32
	deps.Sync = func(context.Context) (string, error) { syncs.Add(1); return "head", nil }
	deps.Probe = func(context.Context) (string, error) {
		if attempts.Add(1) <= 2 {
			return "", fs.ErrNotExist
		}
		return "head", nil
	}
	r := start(t, options, deps)
	for _, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, time.Minute} {
		if got := receive(t, r.poll.resets); got != want {
			t.Fatalf("delay=%v want=%v", got, want)
		}
		r.poll.tick <- time.Time{}
	}
	receive(t, r.poll.resets)
	if syncs.Load() != 1 {
		t.Fatalf("unchanged head synced %d times", syncs.Load())
	}
}

func TestChangedHeadRetriesUntilApplied(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	var syncs atomic.Int32
	events := make(chan string, 16)
	deps.Publish = func(kind string) { events <- kind }
	deps.Probe = func(context.Context) (string, error) { return "new", nil }
	deps.Sync = func(context.Context) (string, error) {
		if syncs.Add(1) < 3 {
			return "old", nil
		}
		return "new", nil
	}
	r := start(t, options, deps)
	for _, want := range []time.Duration{time.Minute, 2 * time.Minute, time.Minute, time.Minute} {
		if got := receive(t, r.poll.resets); got != want {
			t.Fatalf("delay=%v want=%v", got, want)
		}
		r.poll.tick <- time.Time{}
	}
	receive(t, r.poll.resets)
	if syncs.Load() != 3 || len(events) != 6 {
		t.Fatalf("syncs=%d events=%d", syncs.Load(), len(events))
	}
}

func TestWatchDebounceFallbackAndUpdates(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	options.UpdateCheck = true
	scans := make(chan struct{}, 8)
	updates := make(chan struct{}, 8)
	events := make(chan string, 8)
	deps.Scan = func(context.Context) error { scans <- struct{}{}; return nil }
	deps.Update = func(context.Context) (bool, error) { updates <- struct{}{}; return true, nil }
	deps.Publish = func(kind string) { events <- kind }
	r := start(t, options, deps)
	receive(t, r.poll.resets)
	receive(t, updates)
	for range 3 {
		r.hints <- struct{}{}
	}
	if d := receive(t, r.debounce.resets); d != time.Second {
		t.Fatal(d)
	}
	r.debounce.tick <- time.Time{}
	receive(t, scans)
	close(r.hints)
	r.scan.tick <- time.Time{}
	receive(t, scans)
	receive(t, r.scan.resets)
	r.update.tick <- time.Time{}
	receive(t, updates)
	receive(t, r.update.resets)
	if len(events) != 4 {
		t.Fatalf("events=%d", len(events))
	}
}

func TestSyncDoesNotOverlapAndShutdownWaits(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	var syncs, active, maxActive atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	deps.Probe = func(context.Context) (string, error) { return "new", nil }
	deps.Sync = func(context.Context) (string, error) {
		current := active.Add(1)
		maxActive.Store(max(maxActive.Load(), current))
		defer active.Add(-1)
		if syncs.Add(1) > 1 {
			entered <- struct{}{}
			<-release
		}
		return "old", nil
	}
	r := start(t, options, deps)
	receive(t, r.poll.resets)
	r.poll.tick <- time.Time{}
	receive(t, entered)
	r.poll.tick <- time.Time{}
	select {
	case <-entered:
		t.Fatal("overlapping sync")
	case <-time.After(20 * time.Millisecond):
	}
	r.cancel()
	select {
	case err := <-r.done:
		t.Fatalf("returned during active sync: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if maxActive.Load() != 1 {
		t.Fatal("overlapping sync")
	}
}

func TestInvalidOptionsAndDependencies(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	deps.Clock = manualClock{make(chan *manualTimer, 4)}
	for _, o := range []Options{{}, {Poll: MinPoll - 1, Debounce: time.Second}, {Poll: maxPoll + 1, Debounce: time.Second}, {Poll: time.Second, Debounce: time.Minute + 1}, {Poll: time.Second, Debounce: minDebounce - 1}} {
		if err := Run(t.Context(), o, deps); !errors.Is(err, fs.ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*Dependencies){func(d *Dependencies) { d.Clock = nil }, func(d *Dependencies) { d.Probe = nil }, func(d *Dependencies) { d.Sync = nil }, func(d *Dependencies) { d.Scan = nil }, func(d *Dependencies) { d.Publish = nil }, func(d *Dependencies) { d.Log = nil }, func(d *Dependencies) { d.Jitter = nil }, func(d *Dependencies) { d.Update = nil }} {
		copyDeps := deps
		mutate(&copyDeps)
		options.UpdateCheck = true
		if err := Run(t.Context(), options, copyDeps); !errors.Is(err, fs.ErrInvalid) {
			t.Fatal(err)
		}
	}
}

func TestPollErrorsAndJitterBounds(t *testing.T) {
	t.Parallel()
	_, deps := defaults()
	deps.Probe = func(context.Context) (string, error) { return "", nil }
	if _, err := pollOnce(t.Context(), deps, "old"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	deps.Probe = func(context.Context) (string, error) { return "new", nil }
	deps.Sync = func(context.Context) (string, error) { return "", fs.ErrPermission }
	if head, err := pollOnce(t.Context(), deps, "old"); head != "old" || !errors.Is(err, fs.ErrPermission) {
		t.Fatal(head, err)
	}
	deps.Jitter = func(time.Duration) time.Duration { return 0 }
	if got := jitter(deps, time.Second); got < MinPoll {
		t.Fatal(got)
	}
	deps.Jitter = func(time.Duration) time.Duration { return time.Hour }
	if got := jitter(deps, time.Minute); got != time.Minute {
		t.Fatal(got)
	}
	deps.Update = func(context.Context) (bool, error) { return true, fs.ErrPermission }
	deps.Publish = func(string) { t.Error("published failed update") }
	checkUpdate(t.Context(), deps)
}

func TestBootstrapRetryAndBackoffCap(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	options.Poll = time.Second
	var syncs atomic.Int32
	deps.Sync = func(context.Context) (string, error) {
		if syncs.Add(1) == 1 {
			return "", fs.ErrNotExist
		}
		return "head", nil
	}
	deps.Probe = func(context.Context) (string, error) { return "", fs.ErrPermission }
	r := start(t, options, deps)
	receive(t, r.poll.resets)
	r.poll.tick <- time.Time{}
	if delay := receive(t, r.poll.resets); delay != time.Second {
		t.Fatal("bootstrap did not recover", delay)
	}
	for range 12 {
		r.poll.tick <- time.Time{}
		if delay := receive(t, r.poll.resets); delay > maxBackoff || delay < MinPoll {
			t.Fatal("unbounded backoff", delay)
		}
	}
	if syncs.Load() != 2 {
		t.Fatal("bootstrap was not retried", syncs.Load())
	}
}

func TestTrimFollowsTimerWorkOnly(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	options.UpdateCheck = true
	var trims atomic.Int32
	scans := make(chan struct{}, 8)
	deps.Trim = func() { trims.Add(1) }
	deps.Scan = func(context.Context) error { scans <- struct{}{}; return nil }
	deps.Update = func(context.Context) (bool, error) { return false, nil }
	r := start(t, options, deps)
	receive(t, r.poll.resets)
	// A watch-driven scan must not pay for a forced collection per hint.
	r.hints <- struct{}{}
	receive(t, r.debounce.resets)
	r.debounce.tick <- time.Time{}
	receive(t, scans)
	r.poll.tick <- time.Time{}
	receive(t, r.poll.resets)
	r.scan.tick <- time.Time{}
	receive(t, scans)
	receive(t, r.scan.resets)
	r.update.tick <- time.Time{}
	receive(t, r.update.resets)
	// Initial sync, poll, periodic scan and update check each trim once.
	if got := trims.Load(); got != 4 {
		t.Fatalf("trims=%d want 4", got)
	}
}

type replaceable struct {
	done          chan error
	answers       chan bool
	failures      chan error
	logs          chan string
	poll, replace *manualTimer
	timers        []*manualTimer
}

// startReplaceable runs the scheduler with an executable check; answers and
// failures script Replaced, and logs records only executable/restart actions.
func startReplaceable(t *testing.T, options Options, deps Dependencies) replaceable {
	t.Helper()
	options.ExecutableCheck = time.Minute
	clock := manualClock{timers: make(chan *manualTimer, 5)}
	r := replaceable{done: make(chan error, 1), answers: make(chan bool, 4), failures: make(chan error, 4), logs: make(chan string, 16)}
	deps.Clock = clock
	deps.Replaced = func(context.Context) (bool, error) { return <-r.answers, <-r.failures }
	deps.Log = func(action string, err error) {
		if action == "executable" || action == "restart" {
			r.logs <- action + map[bool]string{true: " failed", false: ""}[err != nil]
		}
	}
	go func() { r.done <- Run(t.Context(), options, deps) }()
	for range 5 {
		r.timers = append(r.timers, receive(t, clock.timers))
	}
	r.poll, r.replace = r.timers[0], r.timers[4]
	return r
}

func (r replaceable) check(t *testing.T, replaced bool, err error) {
	t.Helper()
	r.answers <- replaced
	r.failures <- err
	r.replace.tick <- time.Time{}
}

func TestReplacedExecutableReturnsBetweenOperations(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var syncs atomic.Int32
	deps.Probe = func(context.Context) (string, error) { return "new", nil }
	deps.Sync = func(context.Context) (string, error) {
		if syncs.Add(1) == 1 {
			return "old", nil
		}
		entered <- struct{}{}
		<-release
		return "new", nil
	}
	r := startReplaceable(t, options, deps)
	receive(t, r.poll.resets)
	// A missing or unreadable executable is logged and never restarts.
	r.check(t, false, fs.ErrNotExist)
	if d := receive(t, r.replace.resets); d != time.Minute {
		t.Fatal(d)
	}
	if got := receive(t, r.logs); got != "executable failed" {
		t.Fatal(got)
	}
	// An unchanged or identical executable is silent and keeps running.
	r.check(t, false, nil)
	receive(t, r.replace.resets)
	// A confirmed replacement waits for the active sync before returning.
	r.poll.tick <- time.Time{}
	receive(t, entered)
	r.check(t, true, nil)
	select {
	case err := <-r.done:
		t.Fatalf("returned during active sync: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := receive(t, r.done); !errors.Is(err, ErrReplaced) {
		t.Fatalf("err=%v want ErrReplaced", err)
	}
	if got := receive(t, r.logs); got != "restart" || len(r.logs) != 0 || len(r.answers) != 0 {
		t.Fatalf("log=%q, %d more logs, %d unused answers", got, len(r.logs), len(r.answers))
	}
	r.assertStopped(t)
}

func (r replaceable) assertStopped(t *testing.T) {
	t.Helper()
	for _, timer := range r.timers {
		if !timer.stopped.Load() {
			t.Error("timer leaked")
		}
	}
}

func TestExecutableCheckValidation(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	deps.Clock = manualClock{make(chan *manualTimer, 5)}
	for _, check := range []time.Duration{MinPoll - 1, maxPoll + 1, time.Minute} {
		options.ExecutableCheck = check
		if err := Run(t.Context(), options, deps); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("check=%v without Replaced: %v", check, err)
		}
	}
	deps.Replaced = func(context.Context) (bool, error) { return false, nil }
	for _, check := range []time.Duration{-time.Second, MinPoll - 1, maxPoll + 1} {
		options.ExecutableCheck = check
		if err := Run(t.Context(), options, deps); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("check=%v accepted: %v", check, err)
		}
	}
}
