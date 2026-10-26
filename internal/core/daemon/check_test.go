package daemon

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestCheckRecordsNoChangeAndFailure(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	now := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC)
	var ticks atomic.Int32
	deps.Now = func() time.Time { return now.Add(time.Duration(ticks.Add(1)) * time.Minute) }
	records := make(chan state.CheckRecord, 8)
	deps.Checked = func(r state.CheckRecord) { records <- r }
	var syncs atomic.Int32
	deps.Sync = func(context.Context) (string, error) { syncs.Add(1); return strings.Repeat("a", 40), nil }
	var probes atomic.Int32
	deps.Probe = func(context.Context) (string, error) {
		if probes.Add(1) == 1 {
			return strings.Repeat("a", 40), nil
		}
		return "https://secret@private.invalid/token", errors.New("https://credential@private.invalid/?token=secret")
	}
	r := start(t, options, deps)
	initial := receive(t, records)
	receive(t, r.poll.resets)
	r.poll.tick <- now
	ok := receive(t, records)
	receive(t, r.poll.resets)
	wantOK := state.CheckRecord{SchemaVersion: 1, At: initial.At.Add(time.Minute), NextAt: initial.At.Add(2 * time.Minute), IntervalSeconds: 60, Outcome: "ok", Head: strings.Repeat("a", 12)}
	if !reflect.DeepEqual(ok, wantOK) {
		t.Fatalf("no-change=%+v want=%+v", ok, wantOK)
	}
	if syncs.Load() != 1 {
		t.Fatalf("no-change poll overwrote last sync; calls=%d", syncs.Load())
	}
	r.poll.tick <- now.Add(time.Minute)
	failed := receive(t, records)
	receive(t, r.poll.resets)
	wantFailed := state.CheckRecord{SchemaVersion: 1, At: ok.At.Add(time.Minute), NextAt: ok.At.Add(3 * time.Minute), IntervalSeconds: 60, Outcome: "failed", Category: "operation"}
	if !reflect.DeepEqual(failed, wantFailed) {
		t.Fatalf("failed=%+v want=%+v", failed, wantFailed)
	}
	if syncs.Load() != 1 {
		t.Fatal("failed probe ran sync")
	}
}

func TestCheckCategoriesAndHead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want string
	}{{context.Canceled, "canceled"}, {platform.ErrLocked, "busy"}, {fs.ErrInvalid, "invalid"}, {fs.ErrPermission, "operation"}} {
		if got := category(tc.err); got != tc.want {
			t.Fatalf("category=%s want=%s", got, tc.want)
		}
	}
	if shortHead("012abc") != "012abc" || shortHead("https://secret") != "" {
		t.Fatal("unsafe head")
	}
}

func TestCheckFractionalSchedule(t *testing.T) {
	t.Parallel()
	options, deps := defaults()
	options.Poll = 1500 * time.Millisecond
	now := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return now }
	records := make(chan state.CheckRecord, 1)
	deps.Checked = func(r state.CheckRecord) { records <- r }
	r := start(t, options, deps)
	record := receive(t, records)
	receive(t, r.poll.resets)
	if record.IntervalSeconds != 1.5 || !record.NextAt.Equal(now.Add(options.Poll)) {
		t.Fatalf("fractional interval truncated: %+v", record)
	}
}
