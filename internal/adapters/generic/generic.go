package generic

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Agent is the manifest agent kind this adapter serves.
const Agent = "generic"

// Adapter implements target.Adapter for every enabled generic target.
type Adapter struct {
	home string
}

var _ target.Adapter = Adapter{}

// New returns an adapter that expands "~/" home paths below home, the
// applying user's absolute, clean home directory resolved by the caller.
func New(home string) Adapter {
	return Adapter{home: home}
}

// instance is one enabled instance of one generic target.
type instance struct {
	intent     *manifest.Manifest
	repository string
	target     string
	name       string
	dir        string
}

// Declare returns the targets of every enabled generic instance, in target,
// instance and primitive order. Instructions use the shared renderer with
// primitives.instructions.render (concat by default; generic agents have no
// documented import syntax, so import is an error).
func (a Adapter) Declare(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) ([]target.Target, error) {
	if !filepath.IsAbs(a.home) || filepath.Clean(a.home) != a.home {
		return nil, fmt.Errorf("generic: home %q is not absolute and clean", a.home)
	}
	var declared []target.Target
	for _, name := range slices.Sorted(maps.Keys(intent.Targets)) {
		t := intent.Targets[name]
		if t.Agent != Agent || !on(t.Enabled) {
			continue
		}
		for _, instanceName := range slices.Sorted(maps.Keys(t.Instances)) {
			declaration := t.Instances[instanceName]
			if !on(declaration.Enabled) {
				continue
			}
			in := instance{intent: intent, repository: repository, target: name, name: instanceName, dir: a.expand(declaration.Dir)}
			targets, err := a.declareInstance(fsys, in, declaration)
			if err != nil {
				return nil, fmt.Errorf("generic: target %s instance %s: %w", name, instanceName, err)
			}
			declared = append(declared, targets...)
		}
	}
	return declared, nil
}

func (a Adapter) declareInstance(fsys manifest.FileSystem, in instance, declaration manifest.Instance) ([]target.Target, error) {
	var declared []target.Target
	for _, primitive := range slices.Sorted(maps.Keys(declaration.Roots)) {
		if !in.enabled(primitive) {
			continue
		}
		root := a.expand(declaration.Roots[primitive])
		targets, err := in.declarePrimitive(fsys, primitive, root)
		if err != nil {
			return nil, err
		}
		declared = append(declared, targets...)
	}
	return declared, nil
}

func (in instance) declarePrimitive(fsys manifest.FileSystem, primitive, root string) ([]target.Target, error) {
	switch primitive {
	case "instructions":
		return in.instructions(fsys, root)
	case "skills":
		return in.skills(fsys, root)
	case "commands", "agents", "hooks", "rules":
		return in.collection(fsys, primitive, root)
	case "mcp":
		if in.intent.MCP == nil {
			return nil, nil
		}
		unsupported := in.base("mcp", root)
		unsupported.Kind = target.Merged
		unsupported.Warning = "generic: MCP servers have no documented native format for a generic agent; unsupported"
		return []target.Target{unsupported}, nil
	}
	return nil, fmt.Errorf("unknown primitive %q", primitive)
}

func (in instance) instructions(fsys manifest.FileSystem, path string) ([]target.Target, error) {
	if in.intent.Instructions == nil {
		return nil, nil
	}
	strategy, err := instructions.Choose(in.render(), instructions.Concat, instructions.Concat)
	if err != nil {
		return nil, err
	}
	result, err := instructions.Declare(fsys, instructions.Request{
		Instructions: in.intent.Instructions,
		Repository:   in.repository,
		Path:         path,
		Agent:        Agent,
		Strategy:     strategy,
		Owners:       in.owners("instructions"),
	})
	if err != nil {
		return nil, err
	}
	result.Target.InstanceDir = in.dir
	return []target.Target{result.Target}, nil
}

// skills links or copies every child directory of skills.dirs that holds a
// regular SKILL.md file as <root>/<name>. Unmanaged obstacles need adoption.
func (in instance) skills(fsys manifest.FileSystem, root string) ([]target.Target, error) {
	if in.intent.Skills == nil {
		return nil, nil
	}
	var declared []target.Target
	for _, dir := range in.intent.Skills.Dirs {
		entries, err := readDir(fsys, in.source(dir))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			source := filepath.Join(in.source(dir), entry.Name())
			if !entry.IsDir() {
				continue
			}
			skill, err := regular(fsys, filepath.Join(source, "SKILL.md"))
			if err != nil {
				return nil, err
			}
			if !skill {
				continue
			}
			declared = append(declared, in.materialized("skills", filepath.Join(root, entry.Name()), source))
		}
	}
	return declared, nil
}

// collection places every immediate child of dir and per_agent.generic as
// <root>/<name>; per_agent entries replace same-name dir entries. The manifest
// naming a root for a generic agent declares its entries native.
func (in instance) collection(fsys manifest.FileSystem, primitive, root string) ([]target.Target, error) {
	c := in.collectionOf(primitive)
	if c == nil {
		return nil, nil
	}
	sources := map[string]string{}
	for _, dir := range []string{c.Dir, c.PerAgent[Agent]} {
		if dir == "" {
			continue
		}
		entries, err := readDir(fsys, in.source(dir))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			sources[entry.Name()] = filepath.Join(in.source(dir), entry.Name())
		}
	}
	declared := make([]target.Target, 0, len(sources))
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		declared = append(declared, in.materialized(primitive, filepath.Join(root, name), sources[name]))
	}
	return declared, nil
}

func (in instance) collectionOf(primitive string) *manifest.Collection {
	switch primitive {
	case "commands":
		return in.intent.Commands
	case "agents":
		return in.intent.Agents
	case "hooks":
		return in.intent.Hooks
	}
	return in.intent.Rules
}

func (in instance) materialized(primitive, path, source string) target.Target {
	t := in.base(primitive, path)
	t.Kind = target.Materialized
	t.SourcePath = source
	t.Mode = target.ManifestMode(in.intent, in.target, in.name, primitive)
	return t
}

func (in instance) base(primitive, path string) target.Target {
	return target.Target{Path: path, InstanceDir: in.dir, Owners: in.owners(primitive)}
}

// owners identifies the instance as "<target>/<instance>".
func (in instance) owners(primitive string) []state.Owner {
	return []state.Owner{{Primitive: primitive, AgentInstance: in.target + "/" + in.name}}
}

func (in instance) enabled(primitive string) bool {
	t := in.intent.Targets[in.target]
	if enabled := t.Instances[in.name].Primitives[primitive].Enabled; enabled != nil {
		return *enabled
	}
	return on(t.Primitives[primitive].Enabled)
}

// render prefers the instance's instructions render over the target's.
func (in instance) render() string {
	t := in.intent.Targets[in.target]
	if render := t.Instances[in.name].Primitives["instructions"].Render; render != "" {
		return render
	}
	return t.Primitives["instructions"].Render
}

func (in instance) source(relative string) string {
	return filepath.Join(in.repository, filepath.FromSlash(relative))
}

func (a Adapter) expand(home string) string {
	return filepath.Join(a.home, filepath.FromSlash(strings.TrimPrefix(home, "~/")))
}

func on(enabled *bool) bool {
	return enabled == nil || *enabled
}

// readDir treats a missing source directory as empty: removed sources are
// absent desired state, which the planner reconciles.
func readDir(fsys manifest.FileSystem, dir string) ([]fs.DirEntry, error) {
	entries, err := fsys.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	return entries, nil
}

// regular reports whether path is a regular file; only absence is not an error.
func regular(fsys manifest.FileSystem, path string) (bool, error) {
	info, err := fsys.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return info.Mode().IsRegular(), nil
}
