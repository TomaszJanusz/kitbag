package manifest

import (
	"fmt"
	"path/filepath"
)

// validateTree checks names and confinement of entries below declared source
// directories too: a portable directory name cannot hide an unsafe child.
func validateTree(fsys FileSystem, root, path string, ancestors map[string]bool) error {
	if ancestors[path] {
		return fmt.Errorf("symbolic link directory cycle at %s", path)
	}
	ancestors[path] = true
	defer delete(ancestors, path)
	entries, err := fsys.ReadDir(path)
	if err != nil {
		return err
	}
	seen := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if err = portableName(name); err != nil {
			return err
		}
		folded := foldName(name)
		if previous, ok := seen[folded]; ok {
			return fmt.Errorf("case-insensitive name collision: %s and %s", previous, name)
		}
		seen[folded] = name
		resolved, resolveErr := resolvePath(fsys, filepath.Join(path, name))
		if resolveErr != nil {
			return resolveErr
		}
		if !inside(root, resolved) {
			return fmt.Errorf("source escapes repository through a symbolic link: %s", filepath.Join(path, name))
		}
		if err = validateTreeEntry(fsys, root, resolved, ancestors); err != nil {
			return err
		}
	}
	return nil
}

func validateTreeEntry(fsys FileSystem, root, path string, ancestors map[string]bool) error {
	info, err := fsys.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return validateTree(fsys, root, path, ancestors)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported source file type: %s", path)
	}
	return nil
}

func (d *document) pathCollisions() error {
	seen := map[string]string{}
	for _, ref := range d.paths {
		if ref.role == "foreign" || ref.role == "template" {
			continue
		}
		folded := foldName(ref.value)
		if previous, ok := seen[folded]; ok && previous != ref.value {
			return d.fail(ref.key, "case-insensitive path collision: "+previous+" and "+ref.value, nil)
		}
		seen[folded] = ref.value
	}
	return nil
}
