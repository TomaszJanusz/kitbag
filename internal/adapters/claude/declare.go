package claude

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func (c instanceContext) declare(fsys manifest.FileSystem) (Result, error) {
	var result Result
	if c.intent.Instructions != nil && enabled(c.policy("instructions").Enabled) {
		declarations, err := c.instructions(fsys)
		if err != nil {
			return Result{}, err
		}
		result.Targets = append(result.Targets, declarations.Target)
		result.Warnings = append(result.Warnings, declarations.Warnings...)
	}
	if c.intent.Skills != nil && enabled(c.policy("skills").Enabled) {
		skills, err := c.skills(fsys)
		if err != nil {
			return Result{}, err
		}
		result.Targets = append(result.Targets, skills...)
	}
	collections, err := c.collections(fsys)
	if err != nil {
		return Result{}, err
	}
	result.Targets = append(result.Targets, collections...)

	if c.config.Settings != "" {
		d := c.declaration("settings", filepath.Join(c.directory, "settings.json"), target.Merged)
		d.SourcePath = filepath.Join(c.repository, filepath.FromSlash(c.config.Settings))
		d.Merger = Merger{Repository: c.repository}
		d.Preserve = !enabled(c.instance.Settings)
		result.Targets = append(result.Targets, d)
	}
	return result, c.protectSynced(result.Targets)
}

func (c instanceContext) instructions(fsys manifest.FileSystem) (instructions.Result, error) {
	strategy, err := instructions.Choose(c.policy("instructions").Render, instructions.Import, instructions.Import, instructions.Concat)
	if err != nil {
		return instructions.Result{}, err
	}
	d := c.declaration("instructions", c.root("instructions", "CLAUDE.md"), target.Generated)
	rendered, err := instructions.Declare(fsys, instructions.Request{
		Instructions: c.intent.Instructions,
		Repository:   c.repository, Path: d.Path, Agent: "claude", Strategy: strategy, Owners: d.Owners,
	})
	if err != nil {
		return instructions.Result{}, err
	}
	rendered.Target.InstanceDir, rendered.Target.Mode = d.InstanceDir, d.Mode
	return rendered, nil
}

func (c instanceContext) skills(fsys manifest.FileSystem) ([]target.Target, error) {
	var result []target.Target
	for _, dir := range c.intent.Skills.Dirs {
		source := filepath.Join(c.repository, filepath.FromSlash(dir))
		entries, err := fsys.ReadDir(source)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			marker := filepath.Join(source, entry.Name(), "SKILL.md")
			info, statErr := fsys.Lstat(marker)
			if errors.Is(statErr, fs.ErrNotExist) {
				continue
			}
			if statErr != nil {
				return nil, statErr
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if strings.EqualFold(entry.Name(), "synced") {
				return nil, errors.New("skill name 'synced' is reserved by Claude Code")
			}
			d := c.declaration("skills", filepath.Join(c.root("skills", "skills"), entry.Name()), target.Materialized)
			d.SourcePath = filepath.Join(source, entry.Name())
			result = append(result, d)
		}
	}
	return result, nil
}

func (c instanceContext) collection(fsys manifest.FileSystem, primitive string, collection *manifest.Collection) ([]target.Target, error) {
	sources := map[string]string{}
	for _, dir := range []string{collection.Dir, collection.PerAgent["claude"]} {
		if dir == "" {
			continue
		}
		root := filepath.Join(c.repository, filepath.FromSlash(dir))
		entries, err := fsys.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			sources[entry.Name()] = filepath.Join(root, entry.Name())
		}
	}
	result := make([]target.Target, 0, len(sources))
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		d := c.declaration(primitive, filepath.Join(c.root(primitive, primitive), name), target.Materialized)
		d.SourcePath = sources[name]
		native, err := nativeEntry(fsys, d.SourcePath, primitive == "hooks")
		if err != nil {
			return nil, err
		}
		if !native {
			d.Warning = "claude: unsupported native format for " + primitive + ": " + name
		}
		result = append(result, d)
	}
	return result, nil
}

func nativeEntry(fsys manifest.FileSystem, path string, hook bool) (bool, error) {
	info, err := fsys.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode().IsRegular() {
		return hook || filepath.Ext(path) == ".md", nil
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := fsys.ReadDir(path)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		native, readErr := nativeEntry(fsys, filepath.Join(path, entry.Name()), hook)
		if readErr != nil || !native {
			return false, readErr
		}
	}
	return true, nil
}

func (c instanceContext) collections(fsys manifest.FileSystem) ([]target.Target, error) {
	var result []target.Target
	for _, collection := range []struct {
		source *manifest.Collection
		name   string
	}{
		{c.intent.Commands, "commands"}, {c.intent.Agents, "agents"}, {c.intent.Hooks, "hooks"}, {c.intent.Rules, "rules"},
	} {
		if collection.source == nil || !enabled(c.policy(collection.name).Enabled) {
			continue
		}
		declarations, err := c.collection(fsys, collection.name, collection.source)
		if err != nil {
			return nil, err
		}
		result = append(result, declarations...)
	}
	return result, nil
}

// Protect the reserved tree even when a different primitive overrides its root.
func (c instanceContext) protectSynced(declarations []target.Target) error {
	reserved := filepath.Join(c.directory, "skills", "synced")
	for _, d := range declarations {
		if pathContains(reserved, d.Path) || pathContains(d.Path, reserved) {
			return errors.New("claude: destination overlaps reserved skills/synced: " + d.Path)
		}
	}
	return nil
}

func pathContains(parent, child string) bool {
	for {
		if strings.EqualFold(parent, child) {
			return true
		}
		next := filepath.Dir(child)
		if next == child {
			return false
		}
		child = next
	}
}
