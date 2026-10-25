package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/adapters/claude"
	"github.com/TomaszJanusz/kitbag/internal/adapters/codex"
	"github.com/TomaszJanusz/kitbag/internal/adapters/files"
	"github.com/TomaszJanusz/kitbag/internal/adapters/generic"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type adapterFunc func(manifest.FileSystem, string, *manifest.Manifest) ([]target.Target, error)

func (f adapterFunc) Declare(fs manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
	return f(fs, repo, m)
}

func (c *commandLine) adapters(m *manifest.Manifest, warnings *[]string) ([]target.Adapter, error) {
	home, err := c.host.HomeDir()
	if err != nil {
		return nil, err
	}
	adapters := []target.Adapter{files.New(home)}
	seen := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(m.Targets)) {
		t := m.Targets[name]
		if t.Enabled != nil && !*t.Enabled || seen[t.Agent] {
			continue
		}
		seen[t.Agent] = true
		adapter, err := c.agentAdapter(t.Agent, home, warnings)
		if err != nil {
			return nil, err
		}
		adapters = append(adapters, adapter)
	}
	return adapters, nil
}

func (c *commandLine) agentAdapter(agent, home string, warnings *[]string) (target.Adapter, error) {
	switch agent {
	case "claude":
		dir, err := c.host.AgentHome("CLAUDE_CONFIG_DIR", ".claude")
		if err != nil {
			return nil, err
		}
		adapter, err := claude.New(home, dir)
		if err != nil {
			return nil, err
		}
		return adapterFunc(func(fs manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
			r, err := adapter.Plan(fs, repo, m)
			*warnings = append(*warnings, r.Warnings...)
			return r.Targets, err
		}), nil
	case "codex":
		adapter, err := codex.New(c.host)
		if err != nil {
			return nil, err
		}
		return adapterFunc(func(fs manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
			r, err := adapter.Plan(fs, repo, m)
			*warnings = append(*warnings, r.Warnings...)
			return r.Targets, err
		}), nil
	case "generic":
		adapter := generic.New(home)
		return adapterFunc(func(fs manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
			r, err := adapter.Plan(fs, repo, m)
			*warnings = append(*warnings, r.Warnings...)
			return r.Targets, err
		}), nil
	default:
		return nil, fmt.Errorf("agent %q has no adapter in this version", agent)
	}
}
