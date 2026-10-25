// Package undo builds and executes unmanage and revert plans through the apply
// engine, which rechecks every precondition and journals each step. Unchanged
// owned output is discarded (the repository reproduces it); everything else is
// restored from journaled backups or link text, or skipped as a conflict.
package undo

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Options configures execution. Force only applies to revert: it moves a
// changed destination aside to a backup instead of skipping it.
// LegacyRoots lets an adopted link that reaches its source through declared
// legacy roots count as unchanged (plan.LinkReaches).
type Options struct {
	Now         func() time.Time
	LegacyRoots []string
	Force       bool
	DryRun      bool
}

// Outcome is the plan that was (or would be) executed and the apply result.
type Outcome struct {
	Result apply.Result
	Plan   plan.Plan
}

func newPlan(force bool) plan.Plan {
	return plan.Plan{PlanSchema: plan.SchemaVersion, Force: force, Operations: []plan.Operation{}, Observations: []plan.Observation{}}
}

func execute(fsys platform.FS, session *state.Session, p plan.Plan, options Options) (Outcome, error) {
	order(&p)
	outcome := Outcome{Plan: p}
	if options.DryRun {
		return outcome, nil
	}
	if options.Now == nil || options.Now().IsZero() {
		return outcome, fmt.Errorf("undo clock: %w", fs.ErrInvalid)
	}
	var err error
	outcome.Result, err = apply.Execute(fsys, session, p, apply.Options{Now: options.Now, Force: p.Force})
	return outcome, err
}

// order runs discards (descendants first), then outputs and restores, then
// metadata, then skips; each phase by path. One operation exists per path.
func order(p *plan.Plan) {
	slices.SortFunc(p.Operations, func(a, b plan.Operation) int {
		if r := phase(a.Kind) - phase(b.Kind); r != 0 {
			return r
		}
		if phase(a.Kind) == discardPhase {
			return strings.Compare(b.Path, a.Path)
		}
		return strings.Compare(a.Path, b.Path)
	})
	for _, op := range p.Operations {
		p.Observations = append(p.Observations, plan.Observation{Path: op.Path, Classification: op.Classification})
	}
	slices.SortFunc(p.Observations, func(a, b plan.Observation) int { return strings.Compare(a.Path, b.Path) })
}

const (
	discardPhase = iota
	outputPhase
	metadataPhase
	skipPhase
)

func phase(kind plan.Kind) int {
	switch kind {
	case plan.Discard, plan.Remove:
		return discardPhase
	case plan.Unmerge, plan.Restore, plan.Backup, plan.Materialize, plan.Write, plan.Merge:
		return outputPhase
	case plan.Record, plan.Release:
		return metadataPhase
	case plan.Skip:
		return skipPhase
	default:
		return skipPhase
	}
}

// prepare recovers an interrupted step (never in a dry run) and loads state.
func prepare(fsys platform.FS, session *state.Session, options Options) (state.Database, error) {
	if !options.DryRun {
		if err := apply.Recover(fsys, session); err != nil {
			return state.Database{}, err
		}
	}
	db, err := session.Load()
	if err == nil && db.Pending != nil {
		err = fmt.Errorf("an interrupted step is pending; run kitbag sync to recover it: %w", fs.ErrInvalid)
	}
	return db, err
}

// owned reports whether entry is exactly what state recorded for t: a link
// reaching its source (directly or through legacy roots) or the written bytes.
func owned(fsys platform.FS, entry state.Entry, t state.Target, roots []string) (bool, error) {
	if t.Kind == state.Link {
		if entry.Kind != "link" {
			return false, nil
		}
		return plan.LinkReaches(fsys, t.Path, entry.Link, t.SourcePath, roots)
	}
	return (entry.Kind == "file" || entry.Kind == "directory") && entry.Hash == t.WrittenHash, nil
}

func skip(path, warning string) plan.Operation {
	return plan.Operation{Kind: plan.Skip, Path: path, Classification: plan.Conflict, Warning: warning}
}

// inspect returns the destination entry, or a warning for unsafe parents.
func inspect(fsys platform.FS, path string) (state.Entry, string, error) {
	entry, err := plan.Inspect(fsys, path)
	if errors.Is(err, fs.ErrInvalid) {
		return entry, "unsafe destination or parent; left alone", nil
	}
	return entry, "", err
}

// record restores exactly the journaled ownership without touching the entry.
func record(path string, owner *state.Target, expected state.Entry) plan.Operation {
	if owner == nil {
		return plan.Operation{Kind: plan.Release, Path: path, Classification: plan.UpdateNeeded, Expected: &expected}
	}
	return plan.Operation{Kind: plan.Record, Path: path, Classification: plan.UpdateNeeded, Expected: &expected, Restore: &plan.Restoration{Owner: owner}}
}
