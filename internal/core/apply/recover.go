package apply

import (
	"fmt"
	"slices"

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
	complete := s.Action != "output" && current.Kind == ""
	if s.Action == "output" {
		complete = matches(current, *s.After)
	}
	if complete {
		if err = cleanStage(fsys, s.Stage); err != nil {
			return err
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
		if err := check(fsys, s.Backup, s.Before); err != nil {
			return err
		}
		if err := check(fsys, s.Path, state.Entry{}); err != nil {
			return err
		}
		if err := fsys.Rename(s.Backup, s.Path); err != nil {
			return err
		}
	}
	return cleanStage(fsys, s.Stage)
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
	switch s.Action {
	case "output":
		record(db, *s.After)
	case "remove":
		db.Targets = slices.DeleteFunc(db.Targets, func(t state.Target) bool { return t.Path == s.Path })
	case "backup":
	}
	db.History = append(db.History, state.AppliedPlan{ID: s.ID, AppliedAt: s.At, Changes: []state.Change{{Path: s.Path, Previous: journalPrevious(*s)}}})
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
