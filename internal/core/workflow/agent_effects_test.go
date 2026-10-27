package workflow_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type instanceEffectAdapter struct{ home string }

func (a instanceEffectAdapter) Declare(_ manifest.FileSystem, repo string, m *manifest.Manifest) ([]target.Target, error) {
	declarations := make([]target.Target, 0, len(m.Targets["claude"].Instances))
	for name, instance := range m.Targets["claude"].Instances {
		dir := config.Expand(a.home, instance.Dir)
		declarations = append(declarations, target.Target{InstanceDir: dir, Path: filepath.Join(dir, "CLAUDE.md"), SourcePath: filepath.Join(repo, "source"), Kind: target.Materialized, Backup: target.Once, Owners: []state.Owner{{Primitive: "instructions", AgentInstance: name}}})
	}
	return declarations, nil
}

func effectSetup(t *testing.T) *agentFixture {
	t.Helper()
	f := agentSetup(t, agentManifest)
	f.s.ModeFlag = "copy"
	home, err := f.host.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	f.s.Adapters = func(*manifest.Manifest, *[]string) ([]target.Adapter, error) {
		return []target.Adapter{instanceEffectAdapter{home}}, nil
	}
	write(t, f.host, filepath.Join(f.s.Repository, "source"), "shared")
	return f
}

func hasEffect(effects []workflow.AgentEffect, path, kind string) bool {
	for _, effect := range effects {
		if effect.Path == path && effect.Kind == kind {
			return true
		}
	}
	return false
}

func TestAgentPreviewPlansBackupAndRestoreWithoutWriting(t *testing.T) {
	t.Parallel()
	f := effectSetup(t)
	home, err := f.host.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude_2", "CLAUDE.md")
	write(t, f.host, path, "original")
	change := agents.Change{Op: agents.Add, Kind: "claude", Name: "work", Dir: "~/.claude_2"}
	preview, err := f.s.ChangeAgent(t.Context(), change, workflow.AgentOptions{DryRun: true})
	expect(t, err == nil, "preview %v", err)
	expect(t, preview.Manifest == f.path && preview.Directory == filepath.Dir(path), "paths %+v", preview)
	expect(t, hasEffect(preview.Effects, path, "backup") && hasEffect(preview.Effects, path, "materialize"), "effects %+v", preview.Effects)
	data, err := f.host.ReadFile(path)
	expect(t, err == nil && string(data) == "original" && f.manifest(t) == agentManifest, "preview wrote files", string(data))
	applied, err := f.s.ChangeAgent(t.Context(), change, workflow.AgentOptions{Revision: preview.Plan.Revision})
	expect(t, err == nil && applied.Sync.Changed > 0, "apply", err)
	removal, err := f.s.ChangeAgent(t.Context(), agents.Change{Op: agents.Remove, Target: "claude", Name: "work"}, workflow.AgentOptions{DryRun: true})
	expect(t, err == nil && hasEffect(removal.Effects, path, "restore") && !hasEffect(removal.Effects, path, "discard"), "restore", removal)
}

func TestAgentRemovalPreviewDeletesUnchangedAndRetainsChanged(t *testing.T) {
	t.Parallel()
	for _, changed := range []bool{false, true} {
		name := "unchanged"
		if changed {
			name = "changed"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := effectSetup(t)
			_, err := f.s.ChangeAgent(t.Context(), agents.Change{Op: agents.Add, Kind: "claude", Name: "work", Dir: "~/.claude_2"}, workflow.AgentOptions{})
			expect(t, err == nil, "add %v", err)
			home, err := f.host.HomeDir()
			expect(t, err == nil, "home %v", err)
			path := filepath.Join(home, ".claude_2", "CLAUDE.md")
			if changed {
				write(t, f.host, path, "my edit")
			}
			note := filepath.Join(filepath.Dir(path), "private-note")
			write(t, f.host, note, "keep")
			removal, err := f.s.ChangeAgent(t.Context(), agents.Change{Op: agents.Remove, Target: "claude", Name: "work"}, workflow.AgentOptions{DryRun: true})
			kind := "discard"
			if changed {
				kind = "release"
			}
			expect(t, err == nil && hasEffect(removal.Effects, path, kind), "remove", removal)
			expect(t, !hasEffect(removal.Effects, note, "discard") && !hasEffect(removal.Effects, note, "remove"), "unowned file affected %+v", removal.Effects)
		})
	}
}
