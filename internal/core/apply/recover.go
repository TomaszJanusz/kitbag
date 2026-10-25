package apply

import (
	"cmp"
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Recover reconciles a pending step before loading state and building a new plan,
// inside the same lock. Complete outputs acquire ownership; incomplete outputs
// restore the old destination. Changed external content causes a conflict error.
func Recover(fsys platform.FS, session *state.Session) error {
	db, err := session.Load()
	if err != nil || db.Pending == nil {
		return err
	}
	s := db.Pending
	current, err := plan.Inspect(fsys, s.Path)
	if err != nil {
		return err
	}
	complete := current.Kind == ""
	switch s.Action {
	case "output":
		complete = matches(current, *s.After)
	case "restore":
		complete = current == *s.Restored
	}
	if complete {
		for _, reserved := range []string{s.Stage, s.Discard} {
			if err = cleanStage(fsys, reserved); err != nil {
				return err
			}
		}
		return finish(session, &db)
	}
	if err = rollback(fsys, *s, current); err != nil {
		return err
	}
	db.Pending = nil
	return session.Save(db)
}

func rollback(fsys platform.FS, s state.Step, current state.Entry) error {
	if current != s.Before && current.Kind != "" {
		return fmt.Errorf("recover %s: %w", s.Path, errChanged())
	}
	if current.Kind == "" && s.Before.Kind != "" {
		if err := putBack(fsys, s); err != nil {
			return err
		}
	}
	return cleanStage(fsys, s.Stage)
}

// putBack renames the entry moved aside (backup or discard) to its destination.
func putBack(fsys platform.FS, s state.Step) error {
	aside := cmp.Or(s.Backup, s.Discard)
	if err := check(fsys, aside, s.Before); err != nil {
		return err
	}
	if err := check(fsys, s.Path, state.Entry{}); err != nil {
		return err
	}
	return fsys.Rename(aside, s.Path)
}

func cleanStage(fsys platform.FS, path string) error {
	if path == "" {
		return nil
	}
	// Refuse unsafe parents before descending into the reserved staging tree.
	if _, err := plan.Inspect(fsys, path); err != nil {
		return err
	}
	return removeStage(fsys, path)
}

func matches(entry state.Entry, t state.Target) bool {
	if t.Kind == state.Link {
		return entry.Kind == "link" && entry.Link == t.SourcePath
	}
	return (entry.Kind == "file" || entry.Kind == "directory") && entry.Hash == t.WrittenHash
}

func finish(session *state.Session, db *state.Database) error {
	s := db.Pending
	switch {
	case s.After != nil && !s.Release:
		record(db, *s.After)
	case s.Action != "backup":
		release(db, s.Path)
	}
	db.History = append(db.History, state.AppliedPlan{ID: s.ID, Plan: s.Plan, AppliedAt: s.At, Changes: []state.Change{{Path: s.Path, Previous: journalPrevious(*s)}}})
	db.Pending = nil
	return session.Save(*db)
}

func journalPrevious(s state.Step) *state.Previous {
	if s.Before.Kind == "" {
		return nil
	}
	kind := state.Copy
	hash := s.Before.Hash
	if s.Previous != nil && (s.Previous.Kind == state.Link) != (s.Before.Kind == "link") {
		s.Previous = nil
	}
	if s.Previous != nil {
		kind = s.Previous.Kind
		hash = s.Previous.WrittenHash
	}
	if s.Before.Kind == "link" {
		return &state.Previous{Kind: state.Link, WrittenHash: linkHash(hash, s.Before.Link), LinkTarget: s.Before.Link, Managed: s.Previous}
	}
	return &state.Previous{Kind: kind, WrittenHash: hash, BackupPath: s.Backup, Managed: s.Previous}
}

func linkHash(hash, link string) string {
	if hash != "" {
		return hash
	}
	return plan.HashBytes([]byte(link))
}
