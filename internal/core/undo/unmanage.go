package undo

import (
	"bytes"
	"errors"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// Unmanage removes everything KitBag owns per state and restores the original
// entries it displaced. declarations (from plan.Declarations) supply settings
// mergers; a merged target without one is skipped. Changed or unsafe entries are
// conflicts and keep their ownership; nothing unowned is ever deleted.
//
//   - missing: ownership is released;
//   - unchanged link/file/tree: discarded, or replaced by its original backup;
//   - unchanged settings: restored to the original when only KitBag's merge
//     changed it since, else KitBag's hook groups are removed in place.
func Unmanage(fsys platform.FS, session *state.Session, declarations []target.Target, options Options) (Outcome, error) {
	db, err := prepare(fsys, session, options)
	if err != nil {
		return Outcome{Plan: newPlan(false)}, err
	}
	byPath := map[string]target.Target{}
	for _, d := range declarations {
		byPath[d.Path] = d
	}
	p := newPlan(false)
	for _, t := range db.Targets {
		op, opErr := unmanageTarget(fsys, db.History, t, byPath[t.Path])
		if opErr != nil {
			return Outcome{Plan: p}, opErr
		}
		p.Operations = append(p.Operations, op)
	}
	return execute(fsys, session, p, options)
}

func unmanageTarget(fsys platform.FS, history []state.AppliedPlan, t state.Target, d target.Target) (plan.Operation, error) {
	entry, warning, err := inspect(fsys, t.Path)
	switch {
	case err != nil:
		return plan.Operation{}, err
	case warning != "":
		return skip(t.Path, warning), nil
	case entry.Kind == "":
		return record(t.Path, nil, entry), nil
	case !owned(entry, t):
		return skip(t.Path, "changed since KitBag wrote it; left alone"), nil
	}
	original, found, err := originalBackup(fsys, history, t.Path)
	if err != nil {
		return plan.Operation{}, err
	}
	if t.Kind == state.Merged {
		return unmergeTarget(fsys, t, d, entry, original, found)
	}
	if found {
		return restoreOriginal(t.Path, entry, original), nil
	}
	return plan.Operation{Kind: plan.Discard, Path: t.Path, Classification: plan.UpdateNeeded, Expected: &entry}, nil
}

func restoreOriginal(path string, entry state.Entry, original plan.Restoration) plan.Operation {
	return plan.Operation{Kind: plan.Restore, Path: path, Classification: plan.UpdateNeeded, Expected: &entry, Restore: &original}
}

// originalBackup finds the newest journaled backup of an unmanaged entry that
// KitBag displaced at path (a one-time backup or an adoption move) and that
// still holds the journaled bytes.
func originalBackup(fsys platform.FS, history []state.AppliedPlan, path string) (plan.Restoration, bool, error) {
	for i := len(history) - 1; i >= 0; i-- {
		for _, change := range history[i].Changes {
			prev := change.Previous
			if change.Path != path || prev == nil || prev.Managed != nil || prev.BackupPath == "" {
				continue
			}
			restoration, found, err := intactBackup(fsys, prev)
			if err != nil || found {
				return restoration, found, err
			}
		}
	}
	return plan.Restoration{}, false, nil
}

// intactBackup reports whether a journaled backup still holds its bytes.
func intactBackup(fsys platform.FS, prev *state.Previous) (plan.Restoration, bool, error) {
	entry, err := plan.Inspect(fsys, prev.BackupPath)
	if errors.Is(err, fs.ErrInvalid) {
		return plan.Restoration{}, false, nil
	}
	if err != nil {
		return plan.Restoration{}, false, err
	}
	intact := entry.Kind != "" && entry.Kind != "link" && entry.Hash == prev.WrittenHash
	return plan.Restoration{Backup: prev.BackupPath, Entry: entry}, intact, nil
}

func unmergeTarget(fsys platform.FS, t state.Target, d target.Target, entry state.Entry, original plan.Restoration, found bool) (plan.Operation, error) {
	unmerger, ok := d.Merger.(target.Unmerger)
	if d.Kind != target.Merged || !ok || entry.Kind != "file" {
		return skip(t.Path, "removing managed settings needs the declaring adapter; left alone"), nil
	}
	current, err := fsys.ReadFile(t.Path)
	if err != nil {
		return plan.Operation{}, err
	}
	if found {
		reproduced, reproduceErr := reproducible(fsys, d, original.Backup, entry)
		if reproduceErr != nil || reproduced {
			return restoreOriginal(t.Path, entry, original), reproduceErr
		}
	}
	unmerged, err := unmerger.Unmerge(bytes.Clone(current))
	if err != nil {
		return skip(t.Path, "settings cannot be parsed; left alone"), nil
	}
	if bytes.Equal(unmerged, current) {
		return record(t.Path, nil, entry), nil
	}
	written := t
	written.WrittenHash = plan.HashBytes(unmerged)
	output := desired(written, unmerged)
	return plan.Operation{Kind: plan.Unmerge, Path: t.Path, Classification: plan.UpdateNeeded, Expected: &entry, Desired: output}, nil
}

// reproducible reports whether current settings are exactly the merge of the
// original backup with the declared fragment: nothing else changed them since,
// so restoring the original loses no local edit.
func reproducible(fsys platform.FS, d target.Target, backup string, entry state.Entry) (bool, error) {
	original, err := fsys.ReadFile(backup)
	if err != nil {
		return false, err
	}
	fragment := d.Content
	if d.SourcePath != "" {
		if fragment, err = fsys.ReadFile(d.SourcePath); err != nil {
			return false, err
		}
	}
	merged, err := d.Merger.Merge(original, fragment)
	return err == nil && plan.HashBytes(merged) == entry.Hash, nil
}

func desired(t state.Target, content []byte) *plan.Desired {
	return &plan.Desired{Path: t.Path, Kind: target.Merged, WrittenHash: t.WrittenHash, Owners: t.Owners, Content: content}
}
