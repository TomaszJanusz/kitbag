package apply

import (
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

func prepareStep(fsys platform.FS, db *state.Database, op plan.Operation, options Options) (*state.Step, error) {
	at := options.Now().UTC()
	id, err := unusedID(fsys, db, op.Path, at)
	if err != nil {
		return nil, err
	}
	s := &state.Step{Path: op.Path, Before: *op.Expected, ID: id, At: at, Previous: previous(db, op.Path)}
	switch op.Kind {
	case plan.Backup:
		s.Action = "backup"
	case plan.Remove:
		s.Action = "remove"
	case plan.Materialize, plan.Write, plan.Merge:
		if op.Desired == nil {
			return nil, errUsage()
		}
		mode, modeErr := ResolveMode(op.Desired.Mode, options.Mode, "", "", "")
		if modeErr != nil {
			return nil, modeErr
		}
		after := desiredTarget(*op.Desired, state.Kind(mode))
		s.Action = "output"
		s.After = &after
		s.Stage = op.Path + ".kitbag-stage." + id
	case plan.Record, plan.Skip:
		return nil, errUsage()
	default:
		return nil, errUsage()
	}
	if s.Before.Kind != "" {
		s.Backup = op.Path + ".before-kitbag." + id
	}
	return s, nil
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
	db.Targets = slices.DeleteFunc(db.Targets, func(old state.Target) bool { return old.Path == t.Path })
	db.Targets = append(db.Targets, t)
}
