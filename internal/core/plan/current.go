package plan

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

type current struct {
	hash      string
	warning   string
	exists    bool
	directory bool
	repair    bool
	link      bool
}

func (b *builder) current(path string, previous state.Target) (current, error) {
	missing, warning, err := inspectParents(b.fsys, filepath.Dir(path), b.removed)
	if err != nil || warning != "" || missing {
		return current{warning: warning}, err
	}
	if scheduledRemoval(path, b.removed) {
		return current{}, nil
	}
	warning, err = caseAlias(b.fsys, path, b.removed)
	if err != nil || warning != "" {
		return current{warning: warning}, err
	}
	return b.currentEntry(path, previous)
}

func (b *builder) currentEntry(path string, previous state.Target) (current, error) {
	info, err := b.fsys.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return current{}, nil
	}
	if err != nil {
		return current{}, err
	}
	c := current{exists: true, directory: info.IsDir()}
	if info.Mode()&fs.ModeSymlink != 0 {
		return b.currentLink(path, previous)
	}
	c.hash, err = HashTree(b.fsys, path)
	if errors.Is(err, fs.ErrInvalid) {
		c.warning = "unsupported entry in destination"
		return c, nil
	}
	return c, err
}

func (b *builder) currentLink(path string, previous state.Target) (current, error) {
	link, err := b.fsys.ReadLink(path)
	if err != nil {
		return current{}, err
	}
	reaches := false
	if previous.Kind == state.Link {
		if reaches, err = LinkReaches(b.fsys, path, link, previous.SourcePath, b.legacyRoots); err != nil {
			return current{}, err
		}
	}
	if !reaches {
		replaceable, repairErr := b.replaceable(path)
		if repairErr != nil {
			return current{}, repairErr
		}
		if replaceable {
			return current{exists: true, repair: true, link: true}, nil
		}
		return current{exists: true, warning: "destination is a foreign or retargeted symbolic link"}, nil
	}
	// Link identity establishes ownership, even after source changes/deletion;
	// an adopted chain through legacy roots counts as the same link.
	// The effective destination baseline is W, making plans mode-independent.
	return current{exists: true, hash: previous.WrittenHash, link: true}, nil
}

func (b *builder) stale() error {
	for _, previous := range sortedState(b.old) {
		if _, exists := b.desired[previous.Path]; exists {
			continue
		}
		if b.storageReserved(previous.Path) {
			b.skip(previous.Path, Conflict, "recovery storage overlaps a managed target")
			continue
		}
		c, err := b.current(previous.Path, previous)
		if err != nil {
			return err
		}
		if c.warning != "" || (c.exists && c.hash != previous.WrittenHash) {
			b.skip(previous.Path, Conflict, "stale target is no longer unchanged and owned")
			continue
		}
		b.plan.Operations = append(b.plan.Operations, Operation{Kind: Remove, Path: previous.Path, Classification: UpdateNeeded})
		b.observe(previous.Path, UpdateNeeded)
		b.removed[previous.Path] = true
	}
	return nil
}

func sortedState(targets map[string]state.Target) []state.Target {
	result := make([]state.Target, 0, len(targets))
	for _, t := range targets {
		result = append(result, t)
	}
	slices.SortFunc(result, func(a, b state.Target) int { return strings.Compare(a.Path, b.Path) })
	return result
}

func (b *builder) replaceable(path string) (bool, error) {
	replaceable, err := b.fsys.IsManagedLink(path, b.legacyRoots...)
	if err != nil || replaceable || len(b.legacyRoots) == 0 {
		return replaceable, err
	}
	// Explicit legacy migration also mirrors replaceable's dangling-link rule.
	_, err = b.fsys.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	return false, err
}
