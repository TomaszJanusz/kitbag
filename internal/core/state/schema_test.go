package state_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

// A real schema 1 snapshot, as written by the previous release.
const schemaOne = `{
  "targets": [
    {
      "path": "/dest/a",
      "kind": "copy",
      "source_path": "/repo/config",
      "source_hash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "written_hash": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "owners": [{"primitive": "instructions", "agent_instance": "codex"}]
    }
  ],
  "history": [
    {
      "id": "20260102T030405.000000000Z-0",
      "applied_at": "2026-01-02T03:04:05Z",
      "changes": [{"previous": null, "path": "/dest/a"}]
    }
  ],
  "schema_version": 1
}
`

func TestBuiltInSchemaOneMigration(t *testing.T) {
	t.Parallel()
	store, host, path := newStore(t, state.Options{})
	archive := filepath.Join(filepath.Dir(path), "state.v1.json")
	withLock(t, store, func(session *state.Session) error {
		if err := host.WriteAtomic(path, []byte(schemaOne), 0o600); err != nil {
			return err
		}
		db := load(t, session)
		assertMigrated(t, db)
		if _, err := host.ReadFile(archive); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Load archived or rewrote state: %v", err)
		}
		if err := session.Save(db); err != nil {
			return err
		}
		if got, err := host.ReadFile(archive); err != nil || string(got) != schemaOne {
			t.Fatalf("archive = %q, %v", got, err)
		}
		if got, err := host.ReadFile(path); err != nil || !strings.Contains(string(got), `"schema_version": 2`) {
			t.Fatalf("saved = %s, %v", got, err)
		}
		return nil
	})
}

func assertMigrated(t *testing.T, db state.Database) {
	t.Helper()
	if db.SchemaVersion != state.CurrentVersion || len(db.Targets) != 1 || len(db.History) != 1 || db.History[0].PlanID() != db.History[0].ID {
		t.Fatalf("migrated = %+v", db)
	}
}

func TestSchemaOneMigrationRejectsNonObjects(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`null`, `[]`, `{`} {
		if _, err := (state.V1Migration{}).Migrate([]byte(input)); err == nil {
			t.Errorf("Migrate(%s) succeeded", input)
		}
	}
}

func TestHistoryLimitCountsPlans(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, state.Options{HistoryLimit: 2})
	withLock(t, store, func(session *state.Session) error {
		db := snapshot()
		// Three grouped plans of two entries and one ungrouped schema 1 entry.
		for i := range 6 {
			entry := plan(strconv.Itoa(i))
			entry.Plan = "plan-" + strconv.Itoa(i/2)
			db.History = append(db.History, entry)
		}
		db.History = append(db.History, plan("alone"))
		if err := session.Save(db); err != nil {
			return err
		}
		got := load(t, session)
		var ids []string
		for _, entry := range got.History {
			ids = append(ids, entry.ID)
		}
		if strings.Join(ids, ",") != "4,5,alone" {
			t.Fatalf("retained %v", ids)
		}
		return nil
	})
}

func TestAdoptionAndDiscardJournals(t *testing.T) {
	t.Parallel()
	store, _, _ := newStore(t, state.Options{})
	withLock(t, store, func(session *state.Session) error {
		db := snapshot()
		managed := target("/dest/a")
		db.History = []state.AppliedPlan{{ID: "adopt", Plan: "adopt", AppliedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Changes: []state.Change{
			{Path: "/dest/a", Previous: &state.Previous{Kind: state.Copy, WrittenHash: managed.WrittenHash, Managed: &managed}},
			{Path: "/dest/z", Adopted: true},
		}}}
		if err := session.Save(db); err != nil {
			return err
		}
		got := load(t, session).History[0].Changes
		if got[0].Previous.BackupPath != "" || got[0].Previous.Managed == nil || !got[1].Adopted || got[1].Previous != nil {
			t.Fatalf("journal = %+v", got)
		}
		return nil
	})
}

func TestRejectInvalidOriginalBackups(t *testing.T) {
	t.Parallel()
	for _, change := range []state.Change{
		{Path: "/dest/a"},
		{Path: "/dest/a", Adopted: true},
		{Path: "/dest/a", Previous: &state.Previous{Kind: state.Link, WrittenHash: hash("a"), LinkTarget: "/repo/a"}},
	} {
		t.Run(strconv.FormatBool(change.Adopted)+strconv.FormatBool(change.Previous != nil), func(t *testing.T) {
			t.Parallel()
			store, _, _ := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				db.Backups = []state.Change{change}
				if err := session.Save(db); err == nil {
					t.Fatal("invalid original backup accepted")
				}
				return nil
			})
		})
	}
}
