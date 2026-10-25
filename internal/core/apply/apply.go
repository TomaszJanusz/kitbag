package apply

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func errUsage() error { return fmt.Errorf("invalid apply options: %w", fs.ErrInvalid) }

// Execute applies p inside a live state session. Recover must precede planning;
// a pending transaction or mismatched force authorization rejects this plan.
// An error stops execution immediately; reload/recover/replan before retrying.
func Execute(fsys platform.FS, session *state.Session, p plan.Plan, options Options) (Result, error) {
	result := Result{}
	if err := validateOptions(p, options); err != nil {
		return result, err
	}
	db, err := session.Load()
	if err != nil {
		return result, err
	}
	if db.Pending != nil || p.PlanSchema != plan.SchemaVersion {
		return result, errUsage()
	}
	if err = validateStorage(session, p); err != nil {
		return result, err
	}
	return executeOperations(fsys, session, &db, p, options)
}

func validateOptions(p plan.Plan, options Options) error {
	if options.Now == nil || options.Now().IsZero() || p.Force != options.Force {
		return errUsage()
	}
	_, err := ResolveMode("", options.Mode, "", "", "")
	return err
}

func executeOperations(fsys platform.FS, session *state.Session, db *state.Database, p plan.Plan, options Options) (Result, error) {
	result := Result{}
	blocked := map[string]bool{}
	for _, op := range p.Operations {
		if op.Kind == plan.Skip {
			result.skip(op)
			continue
		}
		if blocked[op.Path] {
			continue
		}
		err := executeStep(fsys, session, db, op, options)
		if errors.Is(err, errChanged()) && db.Pending == nil {
			result.Conflicts = append(result.Conflicts, Diagnostic{op.Path, err.Error()})
			blocked[op.Path] = true
			continue
		}
		if err != nil {
			return result, fmt.Errorf("apply %s: %w", op.Path, err)
		}
		result.Changed = append(result.Changed, op)
	}
	return result, nil
}

// changedError has value equality, allowing errors.Is without global sentinels.
type changedError struct{}

func (changedError) Error() string { return "precondition changed; skipped as conflict" }
func errChanged() error            { return changedError{} }

func (r *Result) skip(op plan.Operation) {
	d := Diagnostic{op.Path, op.Warning}
	if op.Classification == plan.Conflict || op.Classification == plan.UnmanagedObstacle {
		if strings.Contains(op.Warning, "instance directory") {
			r.Warnings = append(r.Warnings, d)
		} else {
			r.Conflicts = append(r.Conflicts, d)
		}
	} else {
		r.Warnings = append(r.Warnings, d)
	}
}

func check(fsys platform.FS, path string, expected state.Entry) error {
	current, err := plan.Inspect(fsys, path)
	if err != nil {
		return fmt.Errorf("%w: %w", errChanged(), err)
	}
	if current != expected {
		return errChanged()
	}
	return nil
}

func executeStep(fsys platform.FS, session *state.Session, db *state.Database, op plan.Operation, options Options) error {
	if err := checkOperation(fsys, op); err != nil {
		return err
	}
	if op.Kind == plan.Record {
		record(db, desiredTarget(*op.Desired, existingKind(db, op.Path)))
		return session.Save(*db)
	}
	step, err := prepareStep(fsys, db, op, options)
	if err != nil {
		return err
	}
	db.Pending = step
	if err = session.Save(*db); err != nil {
		return err
	}
	if err = mutate(fsys, *step, op); err != nil {
		if errors.Is(err, errChanged()) {
			return cancelUnstarted(fsys, session, db, err)
		}
		return err
	}
	return finish(session, db)
}

func checkSource(fsys platform.FS, op plan.Operation) error {
	if op.Desired != nil && op.Desired.InstanceDir != "" {
		info, err := fsys.Lstat(op.Desired.InstanceDir)
		if err != nil || !info.IsDir() {
			return errChanged()
		}
	}
	if op.Desired == nil || op.Desired.SourcePath == "" {
		return nil
	}
	got, err := plan.Inspect(fsys, op.Desired.SourcePath)
	if err != nil || got.Kind == "link" || got.Hash != op.Desired.SourceHash {
		return errChanged()
	}
	return nil
}

func cancelUnstarted(fsys platform.FS, session *state.Session, db *state.Database, cause error) error {
	s := db.Pending
	if s.Backup != "" {
		if err := check(fsys, s.Backup, state.Entry{}); err != nil {
			return cause
		}
	}
	if err := cleanStage(fsys, s.Stage); err != nil {
		return errors.Join(cause, err)
	}
	db.Pending = nil
	if err := session.Save(*db); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func checkOperation(fsys platform.FS, op plan.Operation) error {
	if op.Expected == nil {
		return errUsage()
	}
	if op.Kind == plan.Backup {
		if err := backupAvailable(fsys, op.Path); err != nil {
			return err
		}
	}
	if err := check(fsys, op.Path, *op.Expected); err != nil {
		return err
	}
	return checkSource(fsys, op)
}
