package opencode

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// skills places native skill directory copies; link discovery is UNKNOWN.
func (d declaration) skills(root, mode string) ([]target.Target, error) {
	var out []target.Target
	for _, dir := range d.intent.Skills.Dirs {
		source := d.source(dir)
		entries, err := d.fsys.ReadDir(source)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			skill, err := d.isSkill(filepath.Join(source, entry.Name()))
			if err != nil {
				return nil, err
			}
			if skill {
				out = append(out, target.Target{
					Path:        filepath.Join(root, entry.Name()),
					InstanceDir: d.instanceDir(),
					SourcePath:  filepath.Join(source, entry.Name()),
					Kind:        target.Materialized,
					Mode:        mode,
					Owners:      d.owners(primitiveSkills),
				})
			}
		}
	}
	return out, nil
}

// isSkill applies the manifest's rule: a child with a SKILL.md is a skill.
func (d declaration) isSkill(dir string) (bool, error) {
	info, err := d.fsys.Lstat(filepath.Join(dir, "SKILL.md"))
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrInvalid), errors.Is(err, platform.ErrNotDir):
		return false, nil
	case err != nil:
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

// promptSuffix is the native file extension recorded by the inventory.
const promptSuffix = ".md"

func (d declaration) collection(root string, c *manifest.Collection, primitive string) ([]target.Target, error) {
	sources, err := d.collectionSources(c)
	if err != nil {
		return nil, err
	}
	out := make([]target.Target, 0, len(sources))
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		t := target.Target{
			Path:        filepath.Join(root, name),
			InstanceDir: d.instanceDir(),
			Kind:        target.Materialized,
			Mode:        "copy",
			Owners:      d.owners(primitive),
		}
		info, err := d.fsys.Lstat(sources[name])
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() && strings.HasSuffix(name, promptSuffix) {
			t.SourcePath = sources[name]
		} else {
			t.Warning = "opencode: " + primitive + " entry " + name + " is not a Markdown file; not supported"
		}
		out = append(out, t)
	}
	return out, nil
}

// collectionSources maps entry names to sources; per_agent.opencode wins over dir.
func (d declaration) collectionSources(c *manifest.Collection) (map[string]string, error) {
	sources := map[string]string{}
	for _, dir := range []string{c.Dir, c.PerAgent[Agent]} {
		if dir == "" {
			continue
		}
		source := d.source(dir)
		entries, err := d.fsys.ReadDir(source)
		if err != nil {
			return nil, fmt.Errorf("commands: %w", err)
		}
		for _, entry := range entries {
			sources[entry.Name()] = filepath.Join(source, entry.Name())
		}
	}
	return sources, nil
}

func (d declaration) source(relative string) string {
	return filepath.Join(d.repository, filepath.FromSlash(relative))
}
