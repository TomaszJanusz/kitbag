package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

func (m *Manifest) validateSources(fsys FileSystem, root string) error {
	canonical, err := resolvePath(fsys, root)
	if err != nil {
		return m.doc.fail("", "resolve repository: "+err.Error(), err)
	}
	for _, ref := range m.doc.paths {
		if slices.Contains([]string{"home", "foreign", "template"}, ref.role) {
			continue
		}
		resolved, resolveErr := resolvePath(fsys, filepath.Join(canonical, filepath.FromSlash(ref.value)))
		if resolveErr != nil {
			return m.doc.fail(ref.key, "source "+ref.value+": "+resolveErr.Error(), resolveErr)
		}
		if !inside(canonical, resolved) {
			return m.doc.fail(ref.key, "source escapes repository through a symbolic link: "+ref.value, nil)
		}
		if typeErr := checkSourceType(fsys, resolved, ref.role); typeErr != nil {
			return m.doc.fail(ref.key, typeErr.Error(), typeErr)
		}
		if treeErr := validateTreeEntry(fsys, canonical, resolved, map[string]bool{}); treeErr != nil {
			return m.doc.fail(ref.key, treeErr.Error(), treeErr)
		}
	}
	if m.Skills != nil {
		if err = m.validateSkills(fsys, canonical); err != nil {
			return err
		}
	}
	m.doc.fsys = fsys
	m.doc.root = canonical
	return nil
}

func checkSourceType(fsys FileSystem, path, role string) error {
	info, err := fsys.Lstat(path)
	if err != nil {
		return err
	}
	switch {
	case role == "directory" && !info.IsDir():
		return fmt.Errorf("source %s must be a directory", path)
	case role == "file" && !info.Mode().IsRegular():
		return fmt.Errorf("source %s must be a regular file", path)
	case !info.IsDir() && !info.Mode().IsRegular():
		return fmt.Errorf("source %s has unsupported file type", path)
	}
	return nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolvePath walks each component through FS, including components in link
// targets before interpreting '..'. filepath.Clean before this walk would give
// the wrong answer for a link target such as intermediate-link/../file.
func resolvePath(fsys FileSystem, path string) (string, error) {
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	pending := strings.Split(strings.TrimPrefix(path, volume), string(filepath.Separator))
	links := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if slices.Contains([]string{"", "."}, part) {
			continue
		}
		if part == ".." {
			current = filepath.Dir(current)
			continue
		}
		next := filepath.Join(current, part)
		info, err := fsys.Lstat(next)
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			links++
			if links > maxSymlinks {
				return "", fmt.Errorf("too many symbolic links: %w", fs.ErrInvalid)
			}
			var linkErr error
			current, pending, linkErr = readLinkTarget(fsys, next, current, pending)
			if linkErr != nil {
				return "", linkErr
			}
			continue
		}
		if len(pending) > 0 && !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory: %w", next, fs.ErrInvalid)
		}
		current = next
	}
	return current, nil
}

func (m *Manifest) validateSkills(fsys FileSystem, root string) error {
	seen := map[string]string{}
	for i, dir := range m.Skills.Dirs {
		key := fmt.Sprintf("/skills/dirs/%d", i)
		entries, err := fsys.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return m.doc.fail(key, err.Error(), err)
		}
		for _, entry := range entries {
			if err = m.validateSkill(fsys, root, dir, entry.Name(), key, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manifest) validateSkill(fsys FileSystem, root, dir, name, key string, seen map[string]string) error {
	relative := filepath.Join(dir, name, "SKILL.md")
	marker, err := resolvePath(fsys, filepath.Join(root, relative))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
		return nil
	}
	if err != nil {
		return m.doc.fail(key, err.Error(), err)
	}
	if !inside(root, marker) {
		return m.doc.fail(key, "skill escapes repository: "+relative, nil)
	}
	if err = checkSourceType(fsys, marker, "file"); err != nil {
		return m.doc.fail(key, err.Error(), err)
	}
	if err = portableName(name); err != nil {
		return m.doc.fail(key, err.Error(), nil)
	}
	folded := foldName(name)
	if previous, ok := seen[folded]; ok {
		return m.doc.fail(key, "skill name collision: "+relative+" and "+previous, nil)
	}
	seen[folded] = relative
	if folded == "synced" && m.hasClaude() {
		return m.doc.fail(key, "skill name 'synced' is reserved for Claude", nil)
	}
	return nil
}

func (m *Manifest) hasClaude() bool {
	for _, target := range m.Targets {
		if target.Agent == "claude" {
			return true
		}
	}
	return false
}

const maxSymlinks = 40

func readLinkTarget(fsys FileSystem, path, current string, pending []string) (string, []string, error) {
	target, err := fsys.ReadLink(path)
	if err != nil {
		return "", nil, err
	}
	if filepath.IsAbs(target) {
		current = filepath.VolumeName(target) + string(filepath.Separator)
	}
	pending = append(strings.Split(strings.TrimPrefix(target, filepath.VolumeName(target)), string(filepath.Separator)), pending...)
	return current, pending, nil
}

// Missing host additions are optional. Existing ones obey the same confinement
// and file-type rules as other instruction sources, after host substitution.
func (m *Manifest) validateHostAddition() error {
	if m.doc == nil || m.doc.fsys == nil || m.Instructions == nil || m.Instructions.Hosts == "" {
		return nil
	}
	path, err := resolvePath(m.doc.fsys, filepath.Join(m.doc.root, filepath.FromSlash(m.Instructions.Hosts)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return m.doc.fail("/instructions/hosts", err.Error(), err)
	}
	if !inside(m.doc.root, path) {
		return m.doc.fail("/instructions/hosts", "host instructions escape repository", nil)
	}
	if err = checkSourceType(m.doc.fsys, path, "file"); err != nil {
		return m.doc.fail("/instructions/hosts", err.Error(), err)
	}
	return nil
}
