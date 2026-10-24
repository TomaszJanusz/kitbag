package state_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func newStore(t *testing.T, options state.Options) (*state.Store, *fake.Host, string) {
	t.Helper()
	home := filepath.FromSlash("/home/kit")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	store, err := state.New(host, host, options)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := host.Dirs()
	if err != nil {
		t.Fatal(err)
	}
	return store, host, filepath.Join(dirs.State, "state.json")
}

func snapshot() state.Database {
	return state.Database{SchemaVersion: state.CurrentVersion, Targets: []state.Target{target("/dest/z"), target("/dest/a")}}
}

func target(path string) state.Target {
	return state.Target{Path: filepath.FromSlash(path), Kind: state.Copy, SourcePath: filepath.FromSlash("/repo/config"), SourceHash: hash("a"), WrittenHash: hash("b"), Owners: []state.Owner{{Primitive: "instructions", AgentInstance: "codex"}}}
}

func hash(char string) string { return "sha256:" + strings.Repeat(char, 64) }

func withLock(t *testing.T, store *state.Store, fn func(*state.Session) error) {
	t.Helper()
	if err := store.WithLock(fn); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, session *state.Session) state.Database {
	t.Helper()
	db, err := session.Load()
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	before := snapshot()
	withLock(t, store, func(session *state.Session) error {
		if db := load(t, session); db.SchemaVersion != state.CurrentVersion || len(db.Targets) != 0 {
			t.Fatalf("missing state = %+v", db)
		}
		return session.Save(before)
	})
	withLock(t, store, func(session *state.Session) error {
		got := load(t, session)
		if len(got.Targets) != len(before.Targets) || !reflect.DeepEqual(got.Targets[0], before.Targets[1]) {
			t.Fatalf("round trip = %+v", got)
		}
		return nil
	})
	if before.Targets[0].Path != filepath.FromSlash("/dest/z") {
		t.Fatal("Save reordered caller data")
	}
	for name, want := range map[string]fs.FileMode{path: 0o600, filepath.Dir(path): 0o700, filepath.Join(filepath.Dir(path), "state.lock"): 0o600} {
		info, err := host.Stat(name)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("mode %s = %v, %v; want %v", name, info, err, want)
		}
	}
}

func TestLockLifetime(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	other, err := state.New(host, host, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var retained *state.Session
	withLock(t, store, func(session *state.Session) error {
		retained = session
		if err := session.Save(snapshot()); err != nil {
			return err
		}
		err := other.WithLock(func(*state.Session) error { t.Error("entered concurrent operation"); return nil })
		if !errors.Is(err, platform.ErrLocked) || !strings.Contains(err.Error(), "another sync") {
			t.Fatalf("contention = %v", err)
		}
		return nil
	})
	if _, err := retained.Load(); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("retained Load = %v", err)
	}
	if err := retained.Save(snapshot()); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("retained Save = %v", err)
	}
	if _, err := host.Stat(filepath.Join(filepath.Dir(path), "state.lock")); err != nil {
		t.Fatal("lock file removed:", err)
	}
	withLock(t, other, func(session *state.Session) error { load(t, session); return nil })
}

func TestLockReleasedAfterErrorAndPanic(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, state.Options{})
	want := errors.New("caller failed")
	if err := store.WithLock(func(*state.Session) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if got := recover(); got != "caller panic" {
				t.Errorf("panic = %v", got)
			}
		}()
		if err := store.WithLock(func(*state.Session) error { panic("caller panic") }); err != nil {
			t.Fatal(err)
		}
	}()
	withLock(t, store, func(*state.Session) error { return nil })
}

func TestInterruptedWritePreservesSnapshot(t *testing.T) {
	t.Parallel()
	for _, stage := range []fake.WriteStage{fake.PartialWrite, fake.BeforeRename} {
		t.Run(fmt.Sprint(stage), func(t *testing.T) {
			t.Parallel()
			store, host, path := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error { return session.Save(snapshot()) })
			previous, err := host.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := host.InterruptNextWrite(path, stage); err != nil {
				t.Fatal(err)
			}
			withLock(t, store, func(session *state.Session) error {
				if err := session.Save(state.Database{SchemaVersion: state.CurrentVersion}); err == nil {
					t.Fatal("interrupted write succeeded")
				}
				if got, err := host.ReadFile(path); err != nil || !bytes.Equal(got, previous) {
					t.Fatalf("previous bytes lost: %q, %v", got, err)
				}
				if got := load(t, session); len(got.Targets) != len(snapshot().Targets) {
					t.Fatalf("previous snapshot unreadable: %+v", got)
				}
				return session.Save(snapshot()) // One-shot fault; retry succeeds.
			})
		})
	}
}

func plan(id string) state.AppliedPlan {
	previous := target("/dest/old")
	return state.AppliedPlan{ID: id, AppliedAt: time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC), Changes: []state.Change{
		{Path: filepath.FromSlash("/dest/old"), Previous: &state.Previous{Kind: state.Copy, WrittenHash: previous.WrittenHash, BackupPath: filepath.FromSlash("/backups/old"), Managed: &previous}},
		{Path: filepath.FromSlash("/dest/new")},
		{Path: filepath.FromSlash("/dest/link"), Previous: &state.Previous{Kind: state.Link, WrittenHash: hash("c"), LinkTarget: "../repo/old"}},
	}}
}

func TestHistoryBoundAndRevertMetadata(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, 2} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			t.Parallel()
			store, _, _ := newStore(t, state.Options{HistoryLimit: limit})
			want := limit
			if want == 0 {
				want = state.DefaultHistoryLimit
			}
			count := want + 2
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				for i := range count {
					db.History = append(db.History, plan(strconv.Itoa(i)))
				}
				if err := session.Save(db); err != nil {
					return err
				}
				got := load(t, session)
				if len(got.History) != want || got.History[0].ID != "2" || got.History[want-1].ID != strconv.Itoa(count-1) {
					t.Fatalf("history = %+v", got.History)
				}
				assertRevertMetadata(t, got.History[0].Changes)
				return nil
			})
		})
	}
}

func assertRevertMetadata(t *testing.T, changes []state.Change) {
	t.Helper()
	if changes[0].Previous.LinkTarget != "../repo/old" || changes[1].Previous != nil || changes[2].Previous.BackupPath == "" || changes[2].Previous.Managed == nil {
		t.Fatalf("revert metadata lost: %+v", changes)
	}
}
