package plan

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Inspect returns a content/type/link precondition after refusing unsafe parents
// and case aliases. It never follows a final link. Absence has the zero value.
func Inspect(fsys platform.FS, path string) (state.Entry, error) {
	if !validPath(path) {
		return state.Entry{}, invalid("precondition path")
	}
	missing, warning, err := inspectParents(fsys, filepath.Dir(path), nil)
	if err != nil {
		return state.Entry{}, err
	}
	if warning != "" {
		return state.Entry{}, invalid(warning)
	}
	if missing {
		return state.Entry{}, nil
	}
	warning, err = caseAlias(fsys, path, nil)
	if err != nil {
		return state.Entry{}, err
	}
	if warning != "" {
		return state.Entry{}, invalid(warning)
	}
	return inspectEntry(fsys, path)
}

func inspectEntry(fsys platform.FS, path string) (state.Entry, error) {
	info, err := fsys.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state.Entry{}, nil
	}
	if err != nil {
		return state.Entry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		link, linkErr := fsys.ReadLink(path)
		return state.Entry{Kind: "link", Link: link}, linkErr
	}
	hash, err := HashTree(fsys, path)
	kind := "file"
	if info.IsDir() {
		kind = "directory"
	}
	return state.Entry{Kind: kind, Hash: hash}, err
}

func (b *builder) bind() error {
	removed := map[string]bool{}
	for i := range b.plan.Operations {
		op := &b.plan.Operations[i]
		if op.Kind == Skip {
			continue
		}
		missing, warning, err := inspectParents(b.fsys, filepath.Dir(op.Path), removed)
		if err != nil {
			return err
		}
		if warning != "" {
			return invalid(warning)
		}
		expected := state.Entry{}
		if !missing && !scheduledRemoval(op.Path, removed) {
			expected, err = inspectEntry(b.fsys, op.Path)
			if err != nil {
				return err
			}
		}
		op.Expected = &expected
		if op.Kind == Backup || op.Kind == Remove {
			removed[op.Path] = true
		}
	}
	return nil
}
