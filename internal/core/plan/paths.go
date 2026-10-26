package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func invalid(message string) error {
	return fmt.Errorf("invalid plan input: %s: %w", message, fs.ErrInvalid)
}

func validPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, 0)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func overlaps(a, b string) bool { return within(a, b) || within(b, a) }

// inspectParents refuses aliases, case-only spellings and non-directory parents.
// A scheduled stale ancestor removal makes the new subtree virtually missing.
func inspectParents(fsys platform.FS, path string, removed map[string]bool) (bool, string, error) {
	parent := filepath.Dir(path)
	if parent != path {
		missing, warning, err := inspectParents(fsys, parent, removed)
		if err != nil || warning != "" || missing {
			return missing, warning, err
		}
	}
	if scheduledRemoval(path, removed) {
		return true, "", nil
	}
	info, err := fsys.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		warning, caseErr := caseAlias(fsys, path, removed)
		return true, warning, caseErr
	}
	if err != nil {
		return false, "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, linkParent(fsys, path), nil
	}
	if !info.IsDir() {
		return false, "non-directory in destination parents", nil
	}
	warning, err := caseAlias(fsys, path, removed)
	return false, warning, err
}

// linkParent names the linked parent and its target, for example macOS /tmp
// -> private/tmp under a temporary HOME, so that the user can use the real path.
func linkParent(fsys platform.FS, path string) string {
	target, err := fsys.ReadLink(path)
	if err != nil {
		return "symbolic link in destination parents: " + path
	}
	return fmt.Sprintf("symbolic link in destination parents: %s -> %s", path, target)
}

func caseAlias(fsys platform.FS, path string, removed map[string]bool) (string, error) {
	parent := filepath.Dir(path)
	if parent == path {
		return "", nil
	}
	entries, err := fsys.ReadDir(parent)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(path) && strings.EqualFold(entry.Name(), filepath.Base(path)) && !removed[filepath.Join(parent, entry.Name())] {
			return "case-insensitive filesystem collision", nil
		}
	}
	return "", nil
}

func sourceParents(fsys platform.FS, path string) error {
	_, warning, err := inspectParents(fsys, filepath.Dir(path), nil)
	if err != nil {
		return err
	}
	if warning != "" {
		return invalid("source: " + strings.Replace(warning, "destination parents", "source parents", 1))
	}
	return nil
}

func scheduledRemoval(path string, removed map[string]bool) bool {
	for name := range removed {
		if strings.EqualFold(path, name) {
			return true
		}
	}
	return false
}
