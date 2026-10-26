package opencode

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Agent is the manifest agent kind this adapter serves.
const Agent = "opencode"

// Locations from docs/agents.md, documentation snapshot 2026-10-24.
// No installed OpenCode release version was recorded by that inventory.
const (
	HomeVariable = "XDG_CONFIG_HOME"
	DefaultHome  = ".config/opencode"
	// DefaultInstance names the single instance used when a target lists none.
	DefaultInstance = "default"
)

// Primitive names used for roots, policy and owners.
const (
	primitiveInstructions = "instructions"
	primitiveSkills       = "skills"
	primitiveCommands     = "commands"
	primitiveSettings     = "settings"
	primitiveAgents       = "agents"
)

// Adapter declares OpenCode targets. Its directories are resolved once by New, so
// Declare itself never consults the environment.
type Adapter struct {
	home         string
	opencodeHome string
}

// Result holds the declarations plus non-blocking warnings, such as a missing
// host addition or a declared primitive this adapter does not implement.
type Result struct {
	Warnings []string
	Targets  []target.Target
}

// New resolves HOME and the XDG OpenCode configuration directory through env.
func New(env platform.Env) (Adapter, error) {
	home, err := env.HomeDir()
	if err != nil {
		return Adapter{}, fmt.Errorf("opencode: %w", err)
	}
	opencodeHome, err := Home(env)
	if err != nil {
		return Adapter{}, fmt.Errorf("opencode: %w", err)
	}
	if !clean(home) || !clean(opencodeHome) {
		return Adapter{}, errors.New("opencode: home directories must be absolute and clean")
	}
	return Adapter{home: home, opencodeHome: opencodeHome}, nil
}

// Declare implements target.Adapter. Use Plan to also receive the warnings.
func (a Adapter) Declare(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) ([]target.Target, error) {
	result, err := a.Plan(fsys, repository, intent)
	return result.Targets, err
}

// Plan declares every enabled OpenCode target of a validated, host-resolved
// manifest, reading only through fsys. Targets are sorted by name, instances
// by name, so equal inputs give equal results.
func (a Adapter) Plan(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) (Result, error) {
	if a.home == "" {
		return Result{}, errors.New("opencode: adapter not constructed with New")
	}
	if !clean(repository) {
		return Result{}, errors.New("opencode: repository must be absolute and clean")
	}
	var result Result
	for _, name := range slices.Sorted(maps.Keys(intent.Targets)) {
		t := intent.Targets[name]
		if t.Agent != Agent || !enabled(t.Enabled) {
			continue
		}
		instances := a.instances(t)
		for _, instance := range instances {
			d := declaration{fsys: fsys, repository: repository, intent: intent, target: name, instance: instance}
			if err := d.declare(a, &result); err != nil {
				return Result{}, fmt.Errorf("opencode: target %s instance %s: %w", name, instance.name, err)
			}
		}
	}
	return result, nil
}

type instance struct {
	manifest.Instance
	name string
	dir  string
	// listed instances must exist; the default instance may be created.
	listed bool
}

// instances returns the declared instances, or the default OpenCode home. An
// explicit dir always wins over XDG_CONFIG_HOME: it names the directory itself.
func (a Adapter) instances(t manifest.Target) []instance {
	if len(t.Instances) == 0 {
		return []instance{{name: DefaultInstance, dir: a.opencodeHome}}
	}
	var out []instance
	for _, name := range slices.Sorted(maps.Keys(t.Instances)) {
		i := t.Instances[name]
		if enabled(i.Enabled) {
			out = append(out, instance{Instance: i, name: name, dir: a.homePath(i.Dir), listed: true})
		}
	}
	return out
}

type declaration struct {
	fsys       manifest.FileSystem
	intent     *manifest.Manifest
	repository string
	target     string
	instance   instance
}

func (d declaration) declare(a Adapter, result *Result) error {
	if d.on(primitiveInstructions) && d.intent.Instructions != nil {
		if err := d.instructions(a, result); err != nil {
			return err
		}
	}
	if d.on(primitiveSkills) && d.intent.Skills != nil {
		root, mode := a.skillsRoot(d)
		skills, err := d.skills(root, mode)
		if err != nil {
			return err
		}
		result.Targets = append(result.Targets, skills...)
	}
	if err := d.collections(a, result); err != nil {
		return err
	}

	d.settings(result)
	d.unimplemented(result)
	return nil
}

func (d declaration) collections(a Adapter, result *Result) error {
	for _, c := range []struct {
		intent    *manifest.Collection
		primitive string
	}{
		{d.intent.Commands, primitiveCommands}, {d.intent.Agents, primitiveAgents},
	} {
		if !d.on(c.primitive) || c.intent == nil {
			continue
		}
		root := a.root(d.instance, c.primitive, filepath.Join(d.instance.dir, c.primitive))
		entries, err := d.collection(root, c.intent, c.primitive)
		if err != nil {
			return err
		}
		result.Targets = append(result.Targets, entries...)
	}
	return nil
}

// skillsRoot uses native copies because OpenCode skill link discovery is UNKNOWN.
func (a Adapter) skillsRoot(d declaration) (root, mode string) {
	return a.root(d.instance, primitiveSkills, filepath.Join(d.instance.dir, "skills")), "copy"
}

func (d declaration) instructions(a Adapter, result *Result) error {
	strategy, err := instructions.Choose(d.render(), instructions.Concat, instructions.Concat)
	if err != nil {
		return err
	}
	dir := a.root(d.instance, primitiveInstructions, d.instance.dir)
	rendered, err := instructions.Declare(d.fsys, instructions.Request{
		Instructions: d.intent.Instructions,
		Repository:   d.repository,
		Path:         filepath.Join(dir, "AGENTS.md"),
		Agent:        Agent,
		Strategy:     strategy,
		Owners:       d.owners(primitiveInstructions),
	})
	if err != nil {
		return err
	}
	rendered.Target.InstanceDir = d.instanceDir()
	result.Targets = append(result.Targets, rendered.Target)
	result.Warnings = append(result.Warnings, rendered.Warnings...)
	return nil
}

// settings never synchronizes native settings. A declared fragment is reported as
// an unsupported target, so the plan shows it and nothing is written.
func (d declaration) settings(result *Result) {
	t := d.intent.Targets[d.target]
	if t.Settings == "" || (d.instance.Settings != nil && !*d.instance.Settings) {
		return
	}
	result.Targets = append(result.Targets, target.Target{
		Path:        filepath.Join(d.instance.dir, "opencode.json"),
		Kind:        target.Merged,
		InstanceDir: d.instanceDir(),
		Warning:     "opencode: settings are not synchronized; remove targets." + d.target + ".settings",
		Owners:      d.owners(primitiveSettings),
	})
}

// unimplemented reports declared primitives this adapter does not place yet.
func (d declaration) unimplemented(result *Result) {
	for _, p := range []struct {
		name     string
		declared bool
	}{
		{"hooks", d.intent.Hooks != nil},
		{"rules", d.intent.Rules != nil},
		{"mcp", d.intent.MCP != nil},
	} {
		if p.declared && d.on(p.name) {
			result.Warnings = append(result.Warnings, "opencode: "+p.name+" are not synchronized for "+d.owner())
		}
	}
}

// on resolves primitives.<p>.enabled: instance, then target, then true.
func (d declaration) on(primitive string) bool {
	if e := d.instance.Primitives[primitive].Enabled; e != nil {
		return *e
	}
	return enabled(d.intent.Targets[d.target].Primitives[primitive].Enabled)
}

func (d declaration) render() string {
	if r := d.instance.Primitives[primitiveInstructions].Render; r != "" {
		return r
	}
	return d.intent.Targets[d.target].Primitives[primitiveInstructions].Render
}

// instanceDir protects a listed instance that is missing. The default OpenCode
// home may be created when absent.
func (d declaration) instanceDir() string {
	if d.instance.listed {
		return d.instance.dir
	}
	return ""
}

func (d declaration) owner() string {
	return Agent + ":" + d.target + "/" + d.instance.name
}

func (d declaration) owners(primitive string) []state.Owner {
	return []state.Owner{{Primitive: primitive, AgentInstance: d.owner()}}
}

// root returns roots.<primitive> of the instance, otherwise fallback.
func (a Adapter) root(i instance, primitive, fallback string) string {
	if r := i.Roots[primitive]; r != "" {
		return a.homePath(r)
	}
	return fallback
}

// homePath expands a validated "~/" manifest path below the home directory.
func (a Adapter) homePath(path string) string {
	return filepath.Join(a.home, filepath.FromSlash(strings.TrimPrefix(path, "~/")))
}

func enabled(flag *bool) bool {
	return flag == nil || *flag
}

func clean(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}
