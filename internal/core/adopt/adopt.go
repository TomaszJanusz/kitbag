// Package adopt takes over destinations that agent-config-sync (or an earlier
// KitBag) already materialized. Adoption records ownership only: it never
// rewrites, backs up or deletes a destination. An explicit MoveRealDirs request
// is the single exception, replicating the script's --adopt move once.
package adopt

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// Options configures one adoption. Home anchors the MoveRealDirs backup root.
type Options struct {
	Now          func() time.Time
	Home         string
	Mode         string
	MoveRealDirs bool
	DryRun       bool
}

// Adopted is a destination whose ownership adoption records unchanged.
// Legacy marks agent-config-sync's marker, accepted until the output changes.
type Adopted struct {
	Path   string     `json:"path"`
	Kind   state.Kind `json:"kind"`
	Legacy bool       `json:"legacy"`
}

// Move is a real skill directory moved aside by MoveRealDirs.
type Move struct {
	Path   string `json:"path"`
	Backup string `json:"backup"`
}

// Report lists adopted and moved entries, what the next sync would still do
// (Remaining, the fresh plan after adoption) and undeclared legacy links that
// stay untouched. Moves holds the apply result of an explicit move request.
type Report struct {
	Adopted    []Adopted        `json:"adopted"`
	Moved      []Move           `json:"moved"`
	Remaining  []plan.Operation `json:"remaining"`
	Undeclared []string         `json:"undeclared"`
	Moves      apply.Result     `json:"-"`
	DryRun     bool             `json:"dry_run"`
}

// noRootsError has value equality, allowing errors.Is without global sentinels.
type noRootsError struct{}

func (noRootsError) Error() string { return "adoption requires explicit legacy roots" }

// NoLegacyRoots reports whether err is the missing-roots usage error.
func NoLegacyRoots(err error) bool { return errors.Is(err, noRootsError{}) }

// Run adopts inside a live session. req supplies the manifest, adapters, mode
// and the explicit LegacyRoots; its State is loaded here. A dry run only reads.
func Run(fsys platform.FS, session *state.Session, req plan.Request, options Options) (Report, error) {
	report := Report{Adopted: []Adopted{}, Moved: []Move{}, Remaining: []plan.Operation{}, Undeclared: []string{}, DryRun: options.DryRun}
	db, err := load(fsys, session, req, options)
	if err != nil {
		return report, err
	}
	req.State, req.Force = db, false
	if options.MoveRealDirs {
		if err = moveRealDirs(fsys, session, req, options, &report); err != nil {
			return report, err
		}
		if db, err = session.Load(); err != nil {
			return report, err
		}
		req.State = db
	}
	candidates, remaining, err := verified(fsys, req)
	if err != nil {
		return report, err
	}
	report.Remaining = remaining
	if report.Undeclared, err = undeclared(fsys, req); err != nil {
		return report, err
	}
	for _, candidate := range candidates {
		report.Adopted = append(report.Adopted, Adopted{Path: candidate.Target.Path, Kind: candidate.Target.Kind, Legacy: candidate.Legacy})
	}
	if options.DryRun || len(candidates) == 0 {
		return report, nil
	}
	return report, session.Save(adoptedState(db, candidates, options.Now()))
}

// load validates options, recovers an interrupted step (not in a dry run) and
// loads state.
func load(fsys platform.FS, session *state.Session, req plan.Request, options Options) (state.Database, error) {
	if len(req.LegacyRoots) == 0 {
		return state.Database{}, fmt.Errorf("%w: %w", noRootsError{}, fs.ErrInvalid)
	}
	if options.Now == nil || options.Now().IsZero() {
		return state.Database{}, fmt.Errorf("adopt clock: %w", fs.ErrInvalid)
	}
	if !options.DryRun {
		if err := apply.Recover(fsys, session); err != nil {
			return state.Database{}, err
		}
	}
	return session.Load()
}

// verified keeps only candidates that a fresh plan, with them owned, leaves in
// sync. Dropping one can change another (overlap rules), so iterate to a fixpoint.
func verified(fsys platform.FS, req plan.Request) ([]plan.Candidate, []plan.Operation, error) {
	candidates, err := plan.Candidates(fsys, req)
	if err != nil {
		return nil, nil, err
	}
	base := req.State
	for {
		trial := base
		trial.Targets = slices.Clone(base.Targets)
		for _, candidate := range candidates {
			trial.Targets = append(trial.Targets, candidate.Target)
		}
		req.State = trial
		p, buildErr := plan.Build(fsys, req)
		if buildErr != nil {
			return nil, nil, buildErr
		}
		changed := map[string]bool{}
		for _, op := range p.Operations {
			changed[op.Path] = true
		}
		kept := slices.DeleteFunc(slices.Clone(candidates), func(c plan.Candidate) bool { return changed[c.Target.Path] })
		if len(kept) == len(candidates) {
			return candidates, p.Operations, nil
		}
		candidates = kept
	}
}

func adoptedState(db state.Database, candidates []plan.Candidate, now time.Time) state.Database {
	at := now.UTC()
	id := at.Format(apply.BackupTimeLayout) + "-adopt"
	for sequence := 1; slices.ContainsFunc(db.History, func(p state.AppliedPlan) bool { return p.ID == id || p.Plan == id }); sequence++ {
		id = fmt.Sprintf("%s-adopt-%d", at.Format(apply.BackupTimeLayout), sequence)
	}
	entry := state.AppliedPlan{ID: id, Plan: id, AppliedAt: at}
	for _, candidate := range candidates {
		db.Targets = append(db.Targets, candidate.Target)
		entry.Changes = append(entry.Changes, state.Change{Path: candidate.Target.Path, Adopted: true})
	}
	db.History = append(db.History, entry)
	return db
}

// moveRealDirs moves an unowned real directory at a declared skill destination
// to <home>/.agents/skills-before-kitbag/<destination relative to home>, once:
// an existing backup there is never replaced. Moves are journaled through apply.
func moveRealDirs(fsys platform.FS, session *state.Session, req plan.Request, options Options, report *Report) error {
	p, err := movePlan(fsys, req, options.Home)
	if err != nil {
		return err
	}
	for _, op := range p.Operations {
		if op.Kind == plan.Backup {
			report.Moved = append(report.Moved, Move{Path: op.Path, Backup: op.BackupPath})
		}
	}
	if options.DryRun {
		return nil
	}
	report.Moves, err = apply.Execute(fsys, session, p, apply.Options{Now: options.Now, Mode: options.Mode})
	report.Moved = slices.DeleteFunc(report.Moved, func(m Move) bool {
		return !slices.ContainsFunc(report.Moves.Changed, func(op plan.Operation) bool { return op.Path == m.Path })
	})
	return err
}

// movePlan moves every unowned real directory at a declared skill destination.
func movePlan(fsys platform.FS, req plan.Request, home string) (plan.Plan, error) {
	p := plan.Plan{PlanSchema: plan.SchemaVersion, Operations: []plan.Operation{}, Observations: []plan.Observation{}}
	declarations, err := plan.Declarations(fsys, req)
	if err != nil {
		return p, err
	}
	owned := map[string]bool{}
	for _, t := range req.State.Targets {
		owned[t.Path] = true
	}
	for _, d := range declarations {
		if d.Kind != target.Materialized || owned[d.Path] || !slices.ContainsFunc(d.Owners, func(o state.Owner) bool { return o.Primitive == "skills" }) {
			continue
		}
		op, ok, opErr := moveOperation(fsys, home, d.Path)
		if opErr != nil {
			return p, opErr
		}
		if ok {
			p.Operations = append(p.Operations, op)
		}
	}
	return p, nil
}

func moveOperation(fsys platform.FS, home, path string) (plan.Operation, bool, error) {
	entry, err := plan.Inspect(fsys, path)
	if errors.Is(err, fs.ErrInvalid) || entry.Kind != "directory" {
		return plan.Operation{}, false, nil
	}
	if err != nil {
		return plan.Operation{}, false, err
	}
	relative, err := filepath.Rel(home, path)
	if err != nil || !filepath.IsLocal(relative) {
		return skip(path, "real directory outside HOME cannot be moved aside"), true, nil
	}
	backup := filepath.Join(home, ".agents", state.AsideDirectory, relative)
	if _, statErr := fsys.Lstat(backup); !errors.Is(statErr, fs.ErrNotExist) {
		if statErr != nil {
			return plan.Operation{}, false, statErr
		}
		return skip(path, "already moved aside once to "+backup+"; left alone"), true, nil
	}
	return plan.Operation{Kind: plan.Backup, Path: path, Classification: plan.UnmanagedObstacle, BackupPath: backup, Expected: &entry}, true, nil
}

func skip(path, warning string) plan.Operation {
	return plan.Operation{Kind: plan.Skip, Path: path, Classification: plan.UnmanagedObstacle, Warning: warning}
}

// undeclared lists links into legacy roots beside declared destinations that
// the manifest does not declare (for example a skill removed since). They are
// reported and never changed.
func undeclared(fsys platform.FS, req plan.Request) ([]string, error) {
	declarations, err := plan.Declarations(fsys, req)
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	parents := map[string]bool{}
	for _, d := range declarations {
		declared[d.Path] = true
		parents[filepath.Dir(d.Path)] = true
	}
	for _, t := range req.State.Targets {
		declared[t.Path] = true
	}
	result := []string{}
	for _, parent := range sortedKeys(parents) {
		found, listErr := legacyLinks(fsys, parent, declared, req.LegacyRoots)
		if listErr != nil {
			return nil, listErr
		}
		result = append(result, found...)
	}
	return result, nil
}

func legacyLinks(fsys platform.FS, parent string, declared map[string]bool, roots []string) ([]string, error) {
	entries, err := fsys.ReadDir(parent)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, platform.ErrNotDir) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		path := filepath.Join(parent, entry.Name())
		if declared[path] || strings.Contains(entry.Name(), ".before-kitbag.") {
			continue
		}
		legacy, linkErr := fsys.IsManagedLink(path, roots...)
		if linkErr != nil {
			return nil, linkErr
		}
		if legacy {
			result = append(result, path)
		}
	}
	return result, nil
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
