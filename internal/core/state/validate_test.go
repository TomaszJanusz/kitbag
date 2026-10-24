package state_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func TestInvalidSnapshots(t *testing.T) {
	t.Parallel()
	for name, mutate := range invalidSnapshots() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _, _ := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				mutate(&db)
				if err := session.Save(db); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("Save = %v, want invalid", err)
				}
				if got := load(t, session); len(got.Targets) != 0 {
					t.Fatal("invalid snapshot was written")
				}
				return nil
			})
		})
	}
}

func invalidSnapshots() map[string]func(*state.Database) {
	return map[string]func(*state.Database){
		"version":         func(db *state.Database) { db.SchemaVersion++ },
		"relative path":   func(db *state.Database) { db.Targets[0].Path = "relative" },
		"unclean path":    func(db *state.Database) { db.Targets[0].Path = "/dest/../other" },
		"nul path":        func(db *state.Database) { db.Targets[0].Path = "/dest/\x00" },
		"kind":            func(db *state.Database) { db.Targets[0].Kind = "other" },
		"short hash":      func(db *state.Database) { db.Targets[0].WrittenHash = "sha256:a" },
		"uppercase hash":  func(db *state.Database) { db.Targets[0].WrittenHash = hash("A") },
		"nonhex hash":     func(db *state.Database) { db.Targets[0].WrittenHash = hash("z") },
		"algorithm":       func(db *state.Database) { db.Targets[0].WrittenHash = "sha1:abc" },
		"source path":     func(db *state.Database) { db.Targets[0].SourcePath = "relative" },
		"source hash":     func(db *state.Database) { db.Targets[0].SourceHash = "bad" },
		"missing source":  func(db *state.Database) { db.Targets[0].SourcePath = "" },
		"orphan hash":     func(db *state.Database) { db.Targets[0].Kind = state.Generated; db.Targets[0].SourcePath = "" },
		"missing owners":  func(db *state.Database) { db.Targets[0].Owners = nil },
		"empty owner":     func(db *state.Database) { db.Targets[0].Owners[0].Primitive = "" },
		"duplicate owner": func(db *state.Database) { db.Targets[0].Owners = append(db.Targets[0].Owners, db.Targets[0].Owners[0]) },
		"duplicate path":  func(db *state.Database) { db.Targets = append(db.Targets, db.Targets[0]) },
	}
}

func TestInvalidJournals(t *testing.T) {
	t.Parallel()
	for name, mutate := range invalidJournals() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _, _ := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				db.History = []state.AppliedPlan{plan("first")}
				mutate(&db)
				if err := session.Save(db); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("Save = %v, want invalid", err)
				}
				return nil
			})
		})
	}
}

func invalidJournals() map[string]func(*state.Database) {
	return map[string]func(*state.Database){
		"empty ID":     func(db *state.Database) { db.History[0].ID = "" },
		"duplicate ID": func(db *state.Database) { db.History = append(db.History, db.History[0]) },
		"change path":  func(db *state.Database) { db.History[0].Changes[0].Path = "relative" },
		"duplicate change": func(db *state.Database) {
			db.History[0].Changes = append(db.History[0].Changes, db.History[0].Changes[0])
		},
		"kind":          func(db *state.Database) { db.History[0].Changes[0].Previous.Kind = "other" },
		"hash":          func(db *state.Database) { db.History[0].Changes[0].Previous.WrittenHash = "bad" },
		"backup":        func(db *state.Database) { db.History[0].Changes[0].Previous.BackupPath = "relative" },
		"file link":     func(db *state.Database) { db.History[0].Changes[0].Previous.LinkTarget = "../somewhere" },
		"link empty":    func(db *state.Database) { db.History[0].Changes[2].Previous.LinkTarget = "" },
		"link backup":   func(db *state.Database) { db.History[0].Changes[2].Previous.BackupPath = "/backup" },
		"owner path":    func(db *state.Database) { db.History[0].Changes[0].Previous.Managed.Path = "/other" },
		"owner invalid": func(db *state.Database) { db.History[0].Changes[0].Previous.Managed.Owners = nil },
	}
}

func TestAllKindsAndEmptyCollections(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, state.Options{})
	withLock(t, store, func(session *state.Session) error {
		db := state.Database{SchemaVersion: state.CurrentVersion, History: []state.AppliedPlan{plan("empty")}}
		db.History[0].Changes = nil
		for _, kind := range []state.Kind{state.Link, state.Copy, state.Generated, state.Merged} {
			current := target("/dest/" + string(kind))
			current.Kind = kind
			if kind == state.Generated || kind == state.Merged {
				current.SourcePath = ""
				current.SourceHash = ""
			}
			db.Targets = append(db.Targets, current)
		}
		if err := session.Save(db); err != nil {
			return err
		}
		if got := load(t, session); len(got.Targets) != len(db.Targets) || got.History[0].Changes == nil {
			t.Fatalf("round trip = %+v", got)
		}
		return session.Save(state.Database{SchemaVersion: state.CurrentVersion})
	})
}
