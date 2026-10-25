package gemini

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
)

// skills declares one materialized entry per skill below root. Skills keep the
// manifest mode: Gemini CLI documents following skill directory links on Unix.
// The shared root is not an instance directory, so it carries no InstanceDir:
// every Gemini CLI instance declares the same entries and the planner unions owners.
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
					Path:       filepath.Join(root, entry.Name()),
					SourcePath: filepath.Join(source, entry.Name()),
					Kind:       target.Materialized,
					Mode:       mode,
					Owners:     d.owners(primitiveSkills),
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
const promptSuffix = ".toml"

// prompts copies native files unchanged; unsupported formats are never converted.
func (d declaration) prompts(root string) ([]target.Target, error) {
	sources, err := d.commandSources()
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
			Owners:      d.owners(primitiveCommands),
		}
		info, err := d.fsys.Lstat(sources[name])
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() && strings.HasSuffix(name, promptSuffix) {
			t.SourcePath = sources[name]
		} else {
			t.Warning = "gemini: command " + name + " is not a native TOML command file; conversion semantics are unknown; not supported"
		}
		out = append(out, t)
	}
	return out, nil
}

// commandSources maps entry names to sources; per_agent.gemini wins over dir.
func (d declaration) commandSources() (map[string]string, error) {
	sources := map[string]string{}
	for _, dir := range []string{d.intent.Commands.Dir, d.intent.Commands.PerAgent[Agent]} {
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
