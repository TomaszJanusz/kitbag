package state_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestDeterministicOutput(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	withLock(t, store, func(session *state.Session) error {
		db := snapshot()
		db.Targets[0].Owners = append(db.Targets[0].Owners, state.Owner{Primitive: "skills", AgentInstance: "claude"})
		db.Targets = append(db.Targets, state.Target{Path: filepath.FromSlash("/generated"), Kind: state.Generated, WrittenHash: hash("c"), Owners: db.Targets[0].Owners})
		db.History = []state.AppliedPlan{plan("first")}
		if err := session.Save(db); err != nil {
			return err
		}
		first, err := host.ReadFile(path)
		if err != nil {
			return err
		}
		db.Targets[0], db.Targets[1] = db.Targets[1], db.Targets[0]
		owners := db.Targets[1].Owners
		owners[0], owners[1] = owners[1], owners[0]
		changes := db.History[0].Changes
		changes[0], changes[1] = changes[1], changes[0]
		db.History[0].AppliedAt = db.History[0].AppliedAt.In(time.FixedZone("other", 3600))
		if err = session.Save(db); err != nil {
			return err
		}
		second, err := host.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("unstable JSON:\n%s\n%s", first, second)
		}
		if !bytes.HasSuffix(first, []byte("\n")) || !bytes.Contains(first, []byte(`"schema_version": 2`)) {
			t.Fatalf("unexpected JSON %s", first)
		}
		return nil
	})
}

func TestRejectsUnreadableStateWithoutOverwrite(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		`{"schema_version":3,"targets":[]}`, `{"schema_version":0}`, `{"schema_version":-1}`,
		`{}`, `null`, `{"schema_version":1.5}`, `{"schema_version":1,"secret":"no"}`,
		`{"schema_version":2,"secret":"no"}`, `{"schema_version":1} {}`, `{"schema_version":2} {}`,
		`{"schema_version":1,"targets":[{}]}`, `{"schema_version":2,"targets":[{}]}`, `{`,
	} {
		t.Run(data, func(t *testing.T) {
			t.Parallel()
			store, host, path := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				if err := host.WriteAtomic(path, []byte(data), 0o600); err != nil {
					return err
				}
				if _, err := session.Load(); err == nil {
					t.Fatal("invalid state accepted")
				}
				if err := session.Save(snapshot()); err == nil {
					t.Fatal("invalid state overwritten")
				}
				if got, err := host.ReadFile(path); err != nil || string(got) != data {
					t.Fatalf("state changed: %q, %v", got, err)
				}
				return nil
			})
		})
	}
}

type migrationFunc func([]byte) ([]byte, error)

func (fn migrationFunc) Migrate(data []byte) ([]byte, error) { return fn(data) }

func TestMigrationIsExplicitAndPreservesOriginal(t *testing.T) {
	t.Parallel()
	const original = `{"schema_version":1,"legacy":true}`
	migration := migrationFunc(func(data []byte) ([]byte, error) {
		if string(data) != original {
			t.Fatalf("migration input = %s", data)
		}
		return []byte(`{"schema_version":2,"targets":[],"history":[]}`), nil
	})
	store, host, path := newStore(t, state.Options{Migration: migration})
	withLock(t, store, func(session *state.Session) error {
		if err := host.WriteAtomic(path, []byte(original), 0o600); err != nil {
			return err
		}
		if got := load(t, session); got.SchemaVersion != state.CurrentVersion {
			t.Fatal(got)
		}
		if got, err := host.ReadFile(path); err != nil || string(got) != original {
			t.Fatalf("Load rewrote legacy state: %s, %v", got, err)
		}
		if err := session.Save(snapshot()); err != nil {
			return err
		}
		if got, err := host.ReadFile(path); err != nil || strings.Contains(string(got), "legacy") {
			t.Fatalf("explicit save failed: %s, %v", got, err)
		}
		return nil
	})
}

func TestMigrationRejectsBadOutputAndErrors(t *testing.T) {
	t.Parallel()
	for name, migration := range map[string]migrationFunc{
		"error":       func([]byte) ([]byte, error) { return nil, errors.New("migration failed") },
		"no progress": func(data []byte) ([]byte, error) { return data, nil },
		"newer":       func([]byte) ([]byte, error) { return []byte(`{"schema_version":3}`), nil },
		"invalid":     func([]byte) ([]byte, error) { return []byte(`{"schema_version":2,"targets":[{}]}`), nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, host, path := newStore(t, state.Options{Migration: migration})
			withLock(t, store, func(session *state.Session) error {
				if err := host.WriteAtomic(path, []byte(`{"schema_version":1}`), 0o600); err != nil {
					return err
				}
				if _, err := session.Load(); err == nil {
					t.Fatal("bad migration accepted")
				}
				return nil
			})
		})
	}
}

func TestHostErrors(t *testing.T) {
	t.Parallel()
	if _, err := state.New(fake.New(), fake.New(), state.Options{}); err == nil {
		t.Fatal("missing HOME accepted")
	}
	if _, err := state.New(fake.New(), fake.New(), state.Options{HistoryLimit: -1}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	store, host, path := newStore(t, state.Options{})
	withLock(t, store, func(session *state.Session) error {
		if err := session.Save(snapshot()); err != nil {
			return err
		}
		if err := host.Chmod(path, 0); err != nil {
			return err
		}
		if _, err := session.Load(); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("Load = %v", err)
		}
		if err := session.Save(snapshot()); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("Save = %v", err)
		}
		return nil
	})
}

func TestLockAndDirectoryErrors(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	if err := host.WriteAtomic(filepath.FromSlash("/home"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.WithLock(func(*state.Session) error { t.Error("callback entered"); return nil }); !errors.Is(err, platform.ErrNotDir) {
		t.Fatal(err)
	}
	if err := host.Remove(filepath.FromSlash("/home")); err != nil {
		t.Fatal(err)
	}
	withLock(t, store, func(*state.Session) error { return nil })
	lockPath := filepath.Join(filepath.Dir(path), "state.lock")
	if err := host.Chmod(lockPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.WithLock(func(*state.Session) error { t.Error("callback entered"); return nil }); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}

func TestMigrationArchiveAndInterruptedSave(t *testing.T) {
	t.Parallel()
	const original = `{"schema_version":1}`
	migration := migrationFunc(func([]byte) ([]byte, error) { return []byte(`{"schema_version":2}`), nil })
	store, host, path := newStore(t, state.Options{Migration: migration})
	archive := filepath.Join(filepath.Dir(path), "state.v1.json")
	withLock(t, store, func(session *state.Session) error {
		if err := host.WriteAtomic(path, []byte(original), 0o600); err != nil {
			return err
		}
		if err := host.InterruptNextWrite(path, fake.BeforeRename); err != nil {
			return err
		}
		if err := session.Save(snapshot()); err == nil {
			t.Fatal("interrupted migrated save succeeded")
		}
		for _, name := range []string{path, archive} {
			if got, err := host.ReadFile(name); err != nil || string(got) != original {
				t.Fatalf("migration original %s = %s, %v", name, got, err)
			}
		}
		if err := session.Save(snapshot()); err != nil {
			return err
		}
		if got := load(t, session); len(got.Targets) != len(snapshot().Targets) {
			t.Fatal(got)
		}
		return nil
	})
}

func TestMigrationArchiveErrors(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"conflict", "unreadable", "write failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			migration := migrationFunc(func([]byte) ([]byte, error) { return []byte(`{"schema_version":2}`), nil })
			store, host, path := newStore(t, state.Options{Migration: migration})
			archive := filepath.Join(filepath.Dir(path), "state.v1.json")
			withLock(t, store, func(session *state.Session) error {
				const original = `{"schema_version":1}`
				if err := host.WriteAtomic(path, []byte(original), 0o600); err != nil {
					return err
				}
				prepareArchiveError(t, host, archive, scenario)
				if err := session.Save(snapshot()); err == nil {
					t.Fatal("migration archive error ignored")
				}
				if got, err := host.ReadFile(path); err != nil || string(got) != original {
					t.Fatalf("original lost: %s, %v", got, err)
				}
				return nil
			})
		})
	}
}

func prepareArchiveError(t *testing.T, host *fake.Host, archive, scenario string) {
	t.Helper()
	switch scenario {
	case "conflict":
		if err := host.WriteAtomic(archive, []byte("other original"), 0o600); err != nil {
			t.Fatal(err)
		}
	case "unreadable":
		if err := host.WriteAtomic(archive, nil, 0); err != nil {
			t.Fatal(err)
		}
	case "write failure":
		if err := host.InterruptNextWrite(archive, fake.PartialWrite); err != nil {
			t.Fatal(err)
		}
	}
}
