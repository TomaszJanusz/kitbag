package workflow

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// syncPlan builds the sync plan and adds one warning per listed instance
// whose directory it skipped as missing, with the fix ADR 12 gives: restrict
// the instance to the machines that have it. m is the host-resolved manifest.
func (s Service) syncPlan(session *state.Session, m *manifest.Manifest, mode string, force bool, warnings *[]string) (plan.Plan, error) {
	p, err := s.build(session, m, mode, force, warnings)
	if err != nil {
		return p, err
	}
	skipped := missingPaths(p)
	if len(skipped) == 0 {
		return p, nil
	}
	home, err := s.Host.HomeDir()
	if err != nil {
		return p, err
	}
	for _, name := range slices.Sorted(maps.Keys(m.Targets)) {
		for _, key := range missingInstances(home, m.Targets[name], skipped) {
			dir := m.Targets[name].Instances[key].Dir
			*warnings = append(*warnings, fmt.Sprintf("instance %s/%s: %s is missing on this machine; %s", name, key, dir, agents.RestrictHint(name, key)))
		}
	}
	return p, nil
}

// missingPaths lists the operations skipped for a missing instance directory.
func missingPaths(p plan.Plan) []string {
	var paths []string
	for _, op := range p.Operations {
		if op.Kind == plan.Skip && op.Warning == plan.MissingInstance {
			paths = append(paths, op.Path)
		}
	}
	return paths
}

// missingInstances names the enabled listed instances of target that hold a
// skipped path.
func missingInstances(home string, target manifest.Target, skipped []string) []string {
	if target.Enabled != nil && !*target.Enabled {
		return nil
	}
	var names []string
	for _, key := range slices.Sorted(maps.Keys(target.Instances)) {
		instance := target.Instances[key]
		if instance.Enabled != nil && !*instance.Enabled {
			continue
		}
		dir := config.Expand(home, instance.Dir) + string(filepath.Separator)
		if slices.ContainsFunc(skipped, func(path string) bool { return strings.HasPrefix(path, dir) }) {
			names = append(names, key)
		}
	}
	return names
}
