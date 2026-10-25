package state_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func syncRecord() state.SyncRecord {
	return state.SyncRecord{At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("local", 3600)), Repository: filepath.FromSlash("/repo"), SchemaVersion: 1, Result: 0, Changed: 2}
}

func TestSyncRecordLifecycle(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	var retained *state.Session
	withLock(t, store, func(session *state.Session) error {
		retained = session
		_, exists, err := session.LoadSync()
		if err != nil || exists {
			t.Fatalf("missing: %t, %v", exists, err)
		}
		return session.SaveSync(syncRecord())
	})
	if _, _, err := retained.LoadSync(); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("expired LoadSync: %v", err)
	}
	if err := retained.SaveSync(syncRecord()); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("expired SaveSync: %v", err)
	}
	assertSyncMetadata(t, store, host, path)
}

func TestSyncRecordRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{}`, `{"schema_version":2}`, `{"unknown":true}`, `{broken`, `{"schema_version":1,"at":"2026-01-01T00:00:00Z","repository":"relative","result":0,"changed":0}`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			store, host, path := newStore(t, state.Options{})
			file := filepath.Join(filepath.Dir(path), "last-sync.json")
			withLock(t, store, func(session *state.Session) error {
				if err := host.WriteAtomic(file, []byte(raw), 0o600); err != nil {
					return err
				}
				if _, _, err := session.LoadSync(); err == nil {
					t.Error("invalid record accepted")
				}
				if err := session.SaveSync(syncRecord()); err == nil {
					t.Error("invalid record overwritten")
				}
				return nil
			})
			got, err := host.ReadFile(file)
			if err != nil || string(got) != raw {
				t.Fatalf("original lost: %q, %v", got, err)
			}
		})
	}
}

func TestSyncRecordAtomicFailure(t *testing.T) {
	t.Parallel()
	for _, stage := range []fake.WriteStage{fake.PartialWrite, fake.BeforeRename} {
		t.Run(strconv.Itoa(int(stage)), func(t *testing.T) {
			t.Parallel()
			store, host, path := newStore(t, state.Options{})
			file := filepath.Join(filepath.Dir(path), "last-sync.json")
			withLock(t, store, func(session *state.Session) error { return session.SaveSync(syncRecord()) })
			before, err := host.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err = host.InterruptNextWrite(file, stage); err != nil {
				t.Fatal(err)
			}
			withLock(t, store, func(session *state.Session) error {
				record := syncRecord()
				record.Result = 1
				if saveErr := session.SaveSync(record); saveErr == nil {
					t.Error("interruption ignored")
				}
				return nil
			})
			after, err := host.ReadFile(file)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("partial record: %q, %v", after, err)
			}
		})
	}
}

func assertSyncMetadata(t *testing.T, store *state.Store, host *fake.Host, path string) {
	t.Helper()
	withLock(t, store, func(session *state.Session) error {
		r, exists, err := session.LoadSync()
		if err != nil || !exists || r.Result != 0 || r.Changed != 2 || r.At.Location() != time.UTC {
			t.Fatalf("record: %+v, %v", r, err)
		}
		return nil
	})
	if _, err := host.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("sync record rewrote ownership: %v", err)
	}
	info, err := host.Lstat(filepath.Join(filepath.Dir(path), "last-sync.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record privacy: %v, %v", info, err)
	}
}
