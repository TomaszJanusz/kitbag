package plan

import (
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func (b *builder) blockedByStale(path string) bool {
	for old := range b.old {
		if old == path {
			continue
		}
		if _, active := b.desired[old]; !active && foldedOverlap(old, path) && !b.removed[old] {
			return true
		}
	}
	return false
}

func (b *builder) reserved(path string) bool {
	for old := range b.old {
		if foldedOverlap(old, path) {
			return true
		}
	}
	for desired := range b.desired {
		if foldedOverlap(desired, path) {
			return true
		}
	}
	return foldedOverlap(b.repository, path)
}

func compatibleDirectory(fsys platform.FS, p prepared) bool {
	if p.desired.Kind != target.Materialized {
		return false
	}
	info, err := fsys.Lstat(p.desired.SourcePath)
	return err == nil && info.IsDir()
}
