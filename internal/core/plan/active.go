package plan

import (
	"bytes"
	"errors"
	"io/fs"
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func (b *builder) active(p prepared) error {
	d := p.declaration
	if d.Warning != "" {
		b.skip(d.Path, Conflict, d.Warning)
		return nil
	}
	if b.blockedByStale(d.Path) {
		b.skip(d.Path, Conflict, "overlapping stale target cannot be removed")
		return nil
	}
	previous, owned := b.old[d.Path]
	c, err := b.current(d.Path, previous)
	if err != nil {
		return err
	}
	if c.warning != "" {
		b.skip(d.Path, obstacleClass(owned), c.warning)
		return nil
	}
	if b.activeWarning(p, c, owned) {
		return nil
	}
	accepted, err := b.merge(&p, c)
	if err != nil {
		return err
	}
	if !accepted {
		b.skip(d.Path, Conflict, "settings merge rejected input")
		return nil
	}
	if !owned {
		return b.unmanaged(p, c)
	}
	b.managed(p, previous, c)
	return nil
}

func (b *builder) activeWarning(p prepared, c current, owned bool) bool {
	if c.directory && (!owned || !compatibleDirectory(b.fsys, p)) {
		b.skip(p.desired.Path, obstacleClass(owned), "directory obstacle requires explicit adoption")
		return true
	}
	return false
}

func (b *builder) managed(p prepared, previous state.Target, c current) {
	d := p.declaration
	classification := classify(p.desired, previous, c)
	b.observe(d.Path, classification)
	if classification == LocalDrift || classification == Conflict {
		b.plan.Operations = append(b.plan.Operations, Operation{Kind: Skip, Path: d.Path, Classification: classification, Warning: "managed destination has local changes"})
		return
	}
	kind := outputKind(d.Kind)
	if classification == InSync {
		if metadataEqual(p.desired, previous) {
			return
		}
		kind = Record
	}
	b.plan.Operations = append(b.plan.Operations, Operation{Kind: kind, Path: d.Path, Classification: classification, Desired: &p.desired})
}

func obstacleClass(owned bool) Classification {
	if owned {
		return Conflict
	}
	return UnmanagedObstacle
}

func classify(d Desired, previous state.Target, c current) Classification {
	if !c.exists {
		return UpdateNeeded
	}
	// Merge output incorporates allowed local keys; matching it is convergence.
	if d.Kind == target.Merged && c.hash == d.WrittenHash {
		return InSync
	}
	sourceChanged := d.WrittenHash != previous.WrittenHash
	if d.SourcePath != "" {
		sourceChanged = d.SourceHash != previous.SourceHash
	}
	if c.hash == previous.WrittenHash {
		if sourceChanged || outputChanged(d, previous) {
			return UpdateNeeded
		}
		return InSync
	}
	if c.hash == d.WrittenHash {
		return UpdateNeeded
	}
	if sourceChanged {
		return Conflict
	}
	return LocalDrift
}

func sameKind(kind target.Kind, previous state.Kind) bool {
	switch kind {
	case target.Materialized:
		return previous == state.Link || previous == state.Copy
	case target.Generated:
		return previous == state.Generated
	case target.Merged:
		return previous == state.Merged
	default:
		return false
	}
}

func metadataEqual(d Desired, previous state.Target) bool {
	return d.SourcePath == previous.SourcePath && d.SourceHash == previous.SourceHash && d.WrittenHash == previous.WrittenHash && sameKind(d.Kind, previous.Kind) && slices.Equal(d.Owners, sortedOwners(previous.Owners))
}

func outputKind(kind target.Kind) Kind {
	switch kind {
	case target.Materialized:
		return Materialize
	case target.Generated:
		return Write
	case target.Merged:
		return Merge
	default:
		return Skip
	}
}

func (b *builder) merge(p *prepared, c current) (bool, error) {
	if p.desired.Kind != target.Merged {
		return true, nil
	}
	var data []byte
	if c.exists {
		content, err := b.fsys.ReadFile(p.desired.Path)
		if err != nil {
			return false, err
		}
		data = content
	}
	content, err := p.declaration.Merger.Merge(bytes.Clone(data), bytes.Clone(p.desired.Content))
	if err != nil {
		return false, nil
	}
	p.desired.Content = bytes.Clone(content)
	p.desired.WrittenHash = HashBytes(content)
	return true, nil
}

func (b *builder) unmanaged(p prepared, c current) error {
	classification := UpdateNeeded
	if c.exists {
		classification = UnmanagedObstacle
		allowed, err := b.backup(p, c)
		if err != nil || !allowed {
			return err
		}
	}
	b.observe(p.desired.Path, classification)
	b.plan.Operations = append(b.plan.Operations, Operation{Kind: outputKind(p.desired.Kind), Path: p.desired.Path, Classification: classification, Desired: &p.desired})
	return nil
}

func (b *builder) backup(p prepared, c current) (bool, error) {
	path := p.desired.Path
	if p.declaration.Backup != target.Once || c.directory {
		b.skip(path, UnmanagedObstacle, "unmanaged destination requires backup or adoption")
		return false, nil
	}
	backupPath := path + ".kitbag-backup"
	if b.reserved(backupPath) {
		b.skip(path, UnmanagedObstacle, "backup path overlaps a managed target")
		return false, nil
	}
	_, err := b.fsys.Lstat(backupPath)
	if err == nil {
		b.skip(path, UnmanagedObstacle, "one-time backup already exists")
		return false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	b.plan.Operations = append(b.plan.Operations, Operation{Kind: Backup, Path: path, Classification: UnmanagedObstacle, BackupPath: backupPath})
	return true, nil
}

func outputChanged(d Desired, previous state.Target) bool {
	return !sameKind(d.Kind, previous.Kind) || d.SourcePath != previous.SourcePath
}
