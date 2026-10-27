package plan

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type prepared struct {
	desired     Desired
	declaration target.Target
}

type builder struct {
	fsys        platform.FS
	repository  string
	old         map[string]state.Target
	desired     map[string]prepared
	removed     map[string]bool
	machineMode string
	mode        string
	legacyRoots []string
	plan        Plan
	force       bool
}

// Declarations validates req and returns its assembled declarations sorted by
// path, exactly as Build sees them. It never writes.
func Declarations(fsys platform.FS, req Request) ([]target.Target, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	return assemble(fsys, req)
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
		fsys: fsys, repository: req.Repository, force: req.Force, mode: req.Manifest.Mode, machineMode: req.Mode, legacyRoots: slices.Clone(req.LegacyRoots),
		plan: Plan{PlanSchema: SchemaVersion, Force: req.Force, Operations: []Operation{}, Observations: []Observation{}},
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
			if err = b.activate(p); err != nil {
				return Plan{}, fmt.Errorf("plan %s: %w", declaration.Path, err)
			}
		}
	}
	b.sort()
	if err = b.bind(); err != nil {
		return Plan{}, err
	}
	return b.plan, nil
}

func validateRequest(req Request) error {
	if req.Manifest == nil || req.Manifest.Schema < manifest.SchemaOldest || req.Manifest.Schema > manifest.SchemaLatest || len(req.Manifest.Hosts) != 0 {
		return invalid("manifest must be a supported schema and host-resolved")
	}
	if !validPath(req.Repository) || req.State.SchemaVersion != state.CurrentVersion {
		return invalid("repository or state schema")
	}
	if err := validateRequestPolicy(req); err != nil {
		return err
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
		if d.Mode == "" {
			d.Mode = b.manifestMode()
		}
		if err := b.instance(&d); err != nil {
			return err
		}
		desired, err := prepare(b.fsys, d)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if d.Kind == target.Generated {
			desired.LegacyHashes = b.legacyHashes(d.Legacy)
		}
		if errors.Is(err, fs.ErrInvalid) {
			d.Warning = err.Error()
		} else if err != nil {
			return err
		}
		b.desired[d.Path] = prepared{desired: desired, declaration: d}
	}
	return nil
}

func prepare(fsys platform.FS, d target.Target) (Desired, error) {
	desired := Desired{InstanceDir: d.InstanceDir, Mode: d.Mode, Path: d.Path, Kind: d.Kind, SourcePath: d.SourcePath, Owners: sortedOwners(d.Owners), Content: slices.Clone(d.Content)}
	if d.Preserve || d.Warning != "" {
		return desired, nil
	}
	if err := prepareSource(fsys, d, &desired); err != nil {
		return desired, err
	}
	desired.WrittenHash = desired.SourceHash
	if d.Kind != target.Materialized {
		desired.WrittenHash = HashBytes(desired.Content)
	}
	if d.Kind == target.Generated && d.Legacy != nil {
		desired.LegacyHash = HashBytes(d.Legacy)
	}
	return desired, nil
}

// legacyHashes hashes agent-config-sync's rendering for the active clone and,
// with legacy roots configured, for a clone at each legacy root. The rendering
// names the clone only as the repository path (marker, import lines, expanded
// hook commands), so replacing that path gives the old clone's rendering; any
// other mention of the path merely fails to match, keeping the backup.
func (b *builder) legacyHashes(legacy []byte) []string {
	if legacy == nil || len(b.legacyRoots) == 0 {
		return nil
	}
	hashes := []string{HashBytes(legacy)}
	for _, root := range b.legacyRoots {
		if root != b.repository {
			hashes = append(hashes, HashBytes(bytes.ReplaceAll(legacy, []byte(b.repository), []byte(root))))
		}
	}
	return hashes
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
	case Remove, Discard:
		return removePhase
	case Backup:
		return backupPhase
	case Materialize, Write, Merge, Unmerge, Restore:
		return outputPhase
	case Record, Release:
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

func (b *builder) manifestMode() string { return b.mode }

// MissingInstance is the warning of an operation skipped because its listed
// instance directory does not exist on this machine.
const MissingInstance = "instance directory is missing; skipped"

func (b *builder) instance(d *target.Target) error {
	if d.InstanceDir == "" {
		return nil
	}
	info, err := b.fsys.Lstat(d.InstanceDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		d.Warning = MissingInstance
	case err != nil:
		return err
	case !info.IsDir():
		d.Warning = "instance directory is not a real directory; skipped"
	}
	return nil
}

func validateRequestPolicy(req Request) error {
	if req.Mode != "" && req.Mode != "link" && req.Mode != "copy" {
		return invalid("unknown machine mode")
	}
	if req.State.Pending != nil {
		return invalid("recover pending step before planning")
	}
	for _, root := range req.LegacyRoots {
		if !validPath(root) {
			return invalid("invalid legacy root")
		}
	}
	return nil
}

func (b *builder) activate(p prepared) error {
	if p.declaration.Preserve {
		return nil
	}
	return b.active(p)
}
