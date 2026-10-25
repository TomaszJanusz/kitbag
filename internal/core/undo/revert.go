package undo

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Applied is one journaled plan, newest first: Number 1 is the latest.
type Applied struct {
	AppliedAt time.Time `json:"applied_at"`
	ID        string    `json:"id"`
	Paths     []string  `json:"paths"`
	changes   []state.Change
	Number    int  `json:"number"`
	Adopted   bool `json:"adopted"`
}

// History groups the journal into plans, newest first.
func History(db state.Database) []Applied {
	var result []Applied
	for i := len(db.History) - 1; i >= 0; i-- {
		entry := db.History[i]
		if len(result) == 0 || result[len(result)-1].ID != entry.PlanID() {
			result = append(result, Applied{ID: entry.PlanID(), Number: len(result) + 1, Paths: []string{}})
		}
		group := &result[len(result)-1]
		group.AppliedAt = entry.AppliedAt
		// Within a group, later entries come first, so the last seen is oldest.
		for j := len(entry.Changes) - 1; j >= 0; j-- {
			change := entry.Changes[j]
			group.changes = append(group.changes, change)
			group.Adopted = group.Adopted || change.Adopted
			if !slices.Contains(group.Paths, change.Path) {
				group.Paths = append(group.Paths, change.Path)
			}
		}
	}
	for i := range result {
		slices.Sort(result[i].Paths)
	}
	return result
}

// List loads the grouped journal without changing anything.
func List(session *state.Session) ([]Applied, error) {
	db, err := session.Load()
	if err != nil {
		return nil, err
	}
	return History(db), nil
}

// rangeError has value equality, allowing errors.Is without global sentinels.
type rangeError struct{}

func (rangeError) Error() string { return "no such journaled plan" }

// OutOfRange reports a revert request beyond the retained journal.
func OutOfRange(err error) bool { return errors.Is(err, rangeError{}) }

// Revert restores every destination changed by the latest to plans to the entry
// it held before the oldest of them: the journaled previous link text, the
// journaled backup (hash rechecked) or absence, with the previous ownership.
// A destination that no longer matches state is a conflict unless Force moves
// it aside to a backup. Discarded KitBag output cannot be restored; it is
// skipped with a hint to sync. Revert is itself journaled, so it can be undone.
func Revert(fsys platform.FS, session *state.Session, to int, options Options) (Outcome, error) {
	if err := recovered(fsys, session, options); err != nil {
		return Outcome{Plan: newPlan(options.Force)}, err
	}
	db, err := prepare(fsys, session, options)
	if err != nil {
		return Outcome{Plan: newPlan(options.Force)}, err
	}
	plans := History(db)
	if to < 1 || to > len(plans) {
		return Outcome{Plan: newPlan(options.Force)}, fmt.Errorf("%w: revert --to %d, the journal holds %d plan(s): %w", rangeError{}, to, len(plans), fs.ErrInvalid)
	}
	paths, oldest, newer := selected(plans[:to])
	owners := map[string]state.Target{}
	for _, t := range db.Targets {
		owners[t.Path] = t
	}
	p := newPlan(options.Force)
	for _, path := range paths {
		owner, isOwned := owners[path]
		op, opErr := revertPath(fsys, path, oldest[path], newer[path], ownerPointer(owner, isOwned), options)
		if opErr != nil {
			return Outcome{Plan: p}, opErr
		}
		if op != nil {
			p.Operations = append(p.Operations, *op)
		}
	}
	return execute(fsys, session, p, options)
}

// selected returns the sorted paths of plans (newest first) with, for each, the
// oldest change and the change just after it, if any.
func selected(plans []Applied) ([]string, map[string]state.Change, map[string]*state.Change) {
	oldest, newer := map[string]state.Change{}, map[string]*state.Change{}
	var paths []string
	for _, applied := range plans {
		for _, change := range applied.changes {
			if previous, seen := oldest[change.Path]; seen {
				newer[change.Path] = &previous
			} else {
				paths = append(paths, change.Path)
			}
			oldest[change.Path] = change
		}
	}
	slices.Sort(paths)
	return paths, oldest, newer
}

// recovered finishes an interrupted step and then stops: unlike sync and
// unmanage, revert is not idempotent, and the completed steps of an interrupted
// revert form their own journaled plan that a blind retry would undo.
func recovered(fsys platform.FS, session *state.Session, options Options) error {
	db, err := session.Load()
	if err != nil || db.Pending == nil || options.DryRun {
		return err
	}
	if err = apply.Recover(fsys, session); err != nil {
		return err
	}
	return errors.New("recovered an interrupted step; review kitbag revert --list before reverting again " +
		"(the completed steps of an interrupted revert are plan 1, so --to 2 finishes it)")
}

func ownerPointer(t state.Target, ok bool) *state.Target {
	if !ok {
		return nil
	}
	return &t
}

func revertPath(fsys platform.FS, path string, oldest state.Change, newer *state.Change, owner *state.Target, options Options) (*plan.Operation, error) {
	entry, warning, err := inspect(fsys, path)
	if err != nil {
		return nil, err
	}
	if warning != "" {
		op := skip(path, warning)
		return &op, nil
	}
	previous := oldest.Previous
	if oldest.Adopted {
		if newer == nil {
			// Adoption changed no bytes: forgetting ownership restores it.
			op := record(path, nil, entry)
			return &op, nil
		}
		// The adopted bytes are what the next journaled change displaced.
		previous = withoutOwner(newer.Previous)
	}
	unchanged := owner == nil && entry.Kind == ""
	if owner != nil {
		if unchanged, err = owned(fsys, entry, *owner, options.LegacyRoots); err != nil {
			return nil, err
		}
	}
	if !unchanged && !options.Force {
		op := skip(path, "changed since the reverted plan; left alone (use --force to move it aside)")
		return &op, nil
	}
	return restoreTo(fsys, path, entry, previous, !unchanged)
}

func withoutOwner(previous *state.Previous) *state.Previous {
	if previous == nil {
		return nil
	}
	result := *previous
	result.Managed = nil
	return &result
}

func restoreTo(fsys platform.FS, path string, entry state.Entry, previous *state.Previous, aside bool) (*plan.Operation, error) {
	op := plan.Operation{Path: path, Classification: plan.UpdateNeeded, Expected: &entry}
	if aside && entry.Kind != "" {
		op.BackupPath = path + ".before-kitbag."
	}
	switch {
	case previous == nil && entry.Kind == "":
		release := record(path, nil, entry)
		return &release, nil
	case previous == nil:
		op.Kind = plan.Discard
		return &op, nil
	case previous.Kind == state.Link:
		return restoreLink(op, entry, previous), nil
	}
	return restoreFile(fsys, op, entry, previous)
}

// restoreFile renames the journaled backup back. A destination already holding
// those bytes (for example after a later revert) needs only its ownership.
func restoreFile(fsys platform.FS, op plan.Operation, entry state.Entry, previous *state.Previous) (*plan.Operation, error) {
	if (entry.Kind == "file" || entry.Kind == "directory") && entry.Hash == previous.WrittenHash {
		metadata := record(op.Path, previous.Managed, entry)
		return &metadata, nil
	}
	if previous.BackupPath == "" {
		discarded := plan.Operation{Kind: plan.Skip, Path: op.Path, Classification: plan.UpdateNeeded, Warning: "the previous KitBag output was discarded; run kitbag sync to regenerate it"}
		return &discarded, nil
	}
	return restoreBackup(fsys, op, previous)
}

// restoreLink recreates the journaled link text, or only its ownership when the
// destination already holds exactly that link.
func restoreLink(op plan.Operation, entry state.Entry, previous *state.Previous) *plan.Operation {
	restored := state.Entry{Kind: "link", Link: previous.LinkTarget}
	if entry == restored {
		metadata := record(op.Path, previous.Managed, entry)
		return &metadata
	}
	op.Kind, op.Restore = plan.Restore, &plan.Restoration{Link: previous.LinkTarget, Entry: restored, Owner: previous.Managed}
	return &op
}

// restoreBackup renames a journaled backup back, after rechecking its hash.
func restoreBackup(fsys platform.FS, op plan.Operation, previous *state.Previous) (*plan.Operation, error) {
	backup, err := plan.Inspect(fsys, previous.BackupPath)
	if err != nil && !errors.Is(err, fs.ErrInvalid) {
		return nil, err
	}
	if err != nil || backup.Kind == "" || backup.Kind == "link" || backup.Hash != previous.WrittenHash {
		missing := skip(op.Path, "backup "+previous.BackupPath+" is missing or changed; left alone")
		return &missing, nil
	}
	op.Kind, op.Restore = plan.Restore, &plan.Restoration{Backup: previous.BackupPath, Entry: backup, Owner: previous.Managed}
	return &op, nil
}
