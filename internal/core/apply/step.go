package apply

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// BackupTimeLayout is UTC, nanosecond precision, filename-safe on every OS.
const BackupTimeLayout = "20060102T150405.000000000Z"

func prepareStep(fsys platform.FS, db *state.Database, op plan.Operation, options Options, group string) (*state.Step, error) {
	at := options.Now().UTC()
	id, err := unusedID(fsys, db, op.Path, at)
	if err != nil {
		return nil, err
	}
	s := &state.Step{Path: op.Path, Before: *op.Expected, ID: id, At: at, Plan: cmp.Or(group, id), Previous: previous(db, op.Path)}
	if err = prepareAction(s, op, options); err != nil {
		return nil, err
	}
	if s.Before.Kind == "" {
		return s, nil
	}
	return s, setAside(s, op)
}

// setAside chooses where the old entry goes: a journaled backup, the explicit
// adoption directory, or (for unchanged owned output in undo plans) a reserved
// sibling that is deleted once the step completes.
func setAside(s *state.Step, op plan.Operation) error {
	switch {
	case op.Kind == plan.Backup && op.BackupPath != "" && op.BackupPath != op.Path+".before-kitbag.":
		s.Backup = op.BackupPath // explicit adoption move: validated by state
	case undoKind(op.Kind) && op.BackupPath == "":
		if s.Previous == nil || (s.Previous.Kind == state.Link) != (s.Before.Kind == "link") {
			return errChanged()
		}
		s.Discard = op.Path + ".kitbag-stage." + s.ID
		if s.Stage != "" {
			s.Discard += "-old"
		}
	default:
		s.Backup = op.Path + ".before-kitbag." + s.ID
	}
	return nil
}

func undoKind(kind plan.Kind) bool {
	return kind == plan.Discard || kind == plan.Restore || kind == plan.Unmerge
}

func prepareAction(s *state.Step, op plan.Operation, options Options) error {
	switch op.Kind {
	case plan.Backup:
		s.Action = "backup"
	case plan.Remove, plan.Discard:
		s.Action = "remove"
	case plan.Restore:
		if op.Restore == nil {
			return errUsage()
		}
		restored := op.Restore.Entry
		s.Action, s.From, s.Restored, s.After = "restore", op.Restore.Backup, &restored, op.Restore.Owner
	case plan.Materialize, plan.Write, plan.Merge, plan.Unmerge:
		if op.Desired == nil {
			return errUsage()
		}
		mode, modeErr := ResolveMode(op.Desired.Mode, options.Mode, "", "", "")
		if modeErr != nil {
			return modeErr
		}
		after := desiredTarget(*op.Desired, state.Kind(mode))
		s.Action, s.After, s.Release = "output", &after, op.Kind == plan.Unmerge
		s.Stage = op.Path + ".kitbag-stage." + s.ID
	case plan.Record, plan.Release, plan.Skip:
		return errUsage()
	default:
		return errUsage()
	}
	return nil
}

func unusedID(fsys platform.FS, db *state.Database, path string, at time.Time) (string, error) {
	for sequence := 0; ; sequence++ {
		id := fmt.Sprintf("%s-%d", at.Format(BackupTimeLayout), sequence)
		if slices.ContainsFunc(db.History, func(p state.AppliedPlan) bool { return p.ID == id }) {
			continue
		}
		available := true
		for _, suffix := range []string{".before-kitbag.", ".kitbag-stage."} {
			_, err := fsys.Lstat(path + suffix + id)
			if err == nil {
				available = false
			} else if !errors.Is(err, fs.ErrNotExist) {
				return "", err
			}
		}
		if available {
			return id, nil
		}
	}
}

func desiredTarget(d plan.Desired, kind state.Kind) state.Target {
	switch d.Kind {
	case target.Generated:
		kind = state.Generated
	case target.Merged:
		kind = state.Merged
	case target.Materialized:
	}
	return state.Target{Path: d.Path, Kind: kind, SourcePath: d.SourcePath, SourceHash: d.SourceHash, WrittenHash: d.WrittenHash, Owners: d.Owners}
}

func previous(db *state.Database, path string) *state.Target {
	for _, t := range db.Targets {
		if t.Path == path {
			return &t
		}
	}
	return nil
}

func existingKind(db *state.Database, path string) state.Kind {
	if old := previous(db, path); old != nil {
		return old.Kind
	}
	return state.Copy
}

func record(db *state.Database, t state.Target) {
	release(db, t.Path)
	db.Targets = append(db.Targets, t)
}

func release(db *state.Database, path string) {
	db.Targets = slices.DeleteFunc(db.Targets, func(old state.Target) bool { return old.Path == path })
}
