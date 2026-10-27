package claude

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Adapter uses caller-resolved paths, never the process environment. Construct
// it with New after resolving the default directory with Home.
type Adapter struct{ home, directory string }

// Result contains declarations and nonblocking instruction-renderer warnings.
// Use Plan when the caller can display these warnings alongside planner output.
type Result struct {
	Targets  []target.Target
	Warnings []string
}

// New accepts the absolute home and default Claude configuration directory.
// An empty directory selects <home>/.claude; explicit manifest instances always
// use their listed directories and must already exist.
func New(home, directory string) (Adapter, error) {
	if directory == "" {
		directory = filepath.Join(home, ".claude")
	}
	for _, path := range []string{home, directory} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
			return Adapter{}, errors.New("claude: home and default directory must be absolute and clean")
		}
	}
	return Adapter{home, directory}, nil
}

// Declare describes enabled Claude instances from validated, host-resolved
// intent. Reads use fsys exclusively; destination safety and ordering belong to
// the planner. Default instances may be created; listed instances may not.
func (a Adapter) Declare(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) ([]target.Target, error) {
	result, err := a.Plan(fsys, repository, intent)
	return result.Targets, err
}

// Plan also returns nonblocking warnings from the shared instruction renderer.
func (a Adapter) Plan(fsys manifest.FileSystem, repository string, intent *manifest.Manifest) (Result, error) {
	if a.home == "" || intent == nil {
		return Result{}, errors.New("claude: uninitialized adapter or manifest")
	}
	var result Result
	for _, name := range slices.Sorted(maps.Keys(intent.Targets)) {
		config := intent.Targets[name]
		if config.Agent != "claude" || !enabled(config.Enabled) {
			continue
		}
		instances := config.Instances
		instances = defaultInstances(instances)
		for _, instanceName := range slices.Sorted(maps.Keys(instances)) {
			instance := instances[instanceName]
			if !enabled(instance.Enabled) {
				continue
			}
			c := instanceContext{
				adapter: a, repository: repository, intent: intent, config: config,
				instance: instance, name: name, instanceName: instanceName, directory: a.directory,
			}
			if instance.Dir != "" {
				c.directory = a.expand(instance.Dir)
				c.listed = c.directory
			}
			declarations, err := c.declare(fsys)
			if err != nil {
				return Result{}, fmt.Errorf("claude %s/%s: %w", name, instanceName, err)
			}
			result.Targets = append(result.Targets, declarations.Targets...)
			result.Warnings = append(result.Warnings, declarations.Warnings...)
		}
	}
	return result, nil
}

func enabled(value *bool) bool { return value == nil || *value }
func (a Adapter) expand(path string) string {
	return filepath.Join(a.home, filepath.FromSlash(strings.TrimPrefix(path, "~/")))
}

type instanceContext struct {
	adapter                                           Adapter
	intent                                            *manifest.Manifest
	config                                            manifest.Target
	repository, name, instanceName, directory, listed string
	instance                                          manifest.Instance
}

func (c instanceContext) policy(primitive string) manifest.Primitive {
	policy := c.config.Primitives[primitive]
	override := c.instance.Primitives[primitive]
	if override.Enabled != nil {
		policy.Enabled = override.Enabled
	}
	if override.Render != "" {
		policy.Render = override.Render
	}
	return policy
}

func (c instanceContext) root(primitive, fallback string) string {
	if root := c.instance.Roots[primitive]; root != "" {
		return c.adapter.expand(root)
	}
	return filepath.Join(c.directory, fallback)
}

func (c instanceContext) declaration(primitive, path string, kind target.Kind) target.Target {
	return target.Target{
		Path: path, Kind: kind, InstanceDir: c.listed, Backup: target.Once,
		Mode:   target.ManifestMode(c.intent, c.name, c.instanceName, primitive),
		Owners: []state.Owner{{Primitive: primitive, AgentInstance: "claude:" + c.directory}},
	}
}

func defaultInstances(instances map[string]manifest.Instance) map[string]manifest.Instance {
	if len(instances) == 0 {
		return map[string]manifest.Instance{"default": {}}
	}
	return instances
}
