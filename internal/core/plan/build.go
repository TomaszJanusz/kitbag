package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

type prepared struct {
	declaration target.Target
	desired     Desired
}

type builder struct {
	fsys       platform.FS
	repository string
	old        map[string]state.Target
	desired    map[string]prepared
	removed    map[string]bool
	plan       Plan
}

// Build reads the world through fsys and returns a logical plan. It never
// writes files or state. Input errors and unreadable entries fail the whole
// build; unsafe or unsupported individual targets produce skip warnings.
func Build(fsys platform.FS, req Request) (Plan, error) {
	if err := validateRequest(req); err != nil {
		return Plan{}, err
	}
	declarations, err := assemble(fsys, req)
	if err != nil {
		return Plan{}, err
	}
	b := builder{
		fsys: fsys, repository: req.Repository,
		plan: Plan{PlanSchema: SchemaVersion, Operations: []Operation{}, Observations: []Observation{}},
		old:  map[string]state.Target{}, desired: map[string]prepared{}, removed: map[string]bool{},
	}
	for _, previous := range req.State.Targets {
		b.old[previous.Path] = previous
	}
	if err = b.prepare(declarations); err != nil {
		return Plan{}, err
	}
	if err = b.stale(); err != nil {
		return Plan{}, err
	}
	for _, declaration := range declarations {
		p, exists := b.desired[declaration.Path]
		if exists {
			if err = b.active(p); err != nil {
				return Plan{}, fmt.Errorf("plan %s: %w", declaration.Path, err)
			}
		}
	}
	b.sort()
	return b.plan, nil
}

func validateRequest(req Request) error {
	if req.Manifest == nil || req.Manifest.Schema != 1 || len(req.Manifest.Hosts) != 0 {
		return invalid("manifest must be schema 1 and host-resolved")
	}
	if !validPath(req.Repository) || req.State.SchemaVersion != state.CurrentVersion {
		return invalid("repository or state schema")
	}
	return validateStatePaths(req)
}

func validateStatePaths(req Request) error {
	seen := map[string]bool{}
	for _, previous := range req.State.Targets {
		if !validPath(previous.Path) || foldedOverlap(previous.Path, req.Repository) || seen[previous.Path] {
			return invalid("unsafe or duplicate state path: " + previous.Path)
		}
		if !validRecordedSource(previous) {
			return invalid("missing recorded source")
		}
		seen[previous.Path] = true
	}
	return nil
}

func (b *builder) prepare(declarations []target.Target) error {
	for _, d := range declarations {
		desired, err := prepare(b.fsys, d)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if errors.Is(err, fs.ErrInvalid) {
			d.Warning = err.Error()
		} else if err != nil {
			return err
		}
		b.desired[d.Path] = prepared{d, desired}
	}
	return nil
}

func prepare(fsys platform.FS, d target.Target) (Desired, error) {
	desired := Desired{Path: d.Path, Kind: d.Kind, SourcePath: d.SourcePath, Owners: sortedOwners(d.Owners), Content: slices.Clone(d.Content)}
	if d.Warning != "" {
		return desired, nil
	}
	if err := prepareSource(fsys, d, &desired); err != nil {
		return desired, err
	}
	desired.WrittenHash = desired.SourceHash
	if d.Kind != target.Materialized {
		desired.WrittenHash = HashBytes(desired.Content)
	}
	return desired, nil
}

func (b *builder) skip(path string, classification Classification, warning string) {
	b.plan.Operations = append(b.plan.Operations, Operation{Kind: Skip, Path: path, Classification: classification, Warning: warning})
	b.observe(path, classification)
}

func (b *builder) observe(path string, classification Classification) {
	b.plan.Observations = append(b.plan.Observations, Observation{Path: path, Classification: classification})
}

func (b *builder) sort() {
	slices.SortFunc(b.plan.Operations, func(a, c Operation) int {
		if rank := operationRank(a.Kind) - operationRank(c.Kind); rank != 0 {
			return rank
		}
		// Remove descendants before ancestors; all removals precede creations.
		if a.Kind == Remove && a.Path != c.Path && overlaps(a.Path, c.Path) {
			return strings.Compare(c.Path, a.Path)
		}
		return strings.Compare(a.Path, c.Path)
	})
	slices.SortFunc(b.plan.Observations, func(a, c Observation) int { return strings.Compare(a.Path, c.Path) })
}

func operationRank(kind Kind) int {
	switch kind {
	case Remove:
		return removePhase
	case Backup:
		return backupPhase
	case Materialize, Write, Merge:
		return outputPhase
	case Record:
		return recordPhase
	case Skip:
		return skipPhase
	default:
		return unknownPhase
	}
}

func validRecordedSource(previous state.Target) bool {
	return (previous.Kind != state.Link && previous.Kind != state.Copy) || validPath(previous.SourcePath)
}

func prepareSource(fsys platform.FS, d target.Target, desired *Desired) error {
	if d.SourcePath == "" {
		return nil
	}
	if err := sourceParents(fsys, d.SourcePath); err != nil {
		return err
	}
	hash, err := HashTree(fsys, d.SourcePath)
	if err != nil {
		return err
	}
	desired.SourceHash = hash
	if d.Kind == target.Merged {
		desired.Content, err = fsys.ReadFile(d.SourcePath)
	}
	return err
}

const (
	removePhase = iota
	backupPhase
	outputPhase
	recordPhase
	skipPhase
	unknownPhase
)
