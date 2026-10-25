package initialize

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/claude"
	"github.com/TomaszJanusz/kitbag/internal/adapters/codex"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestScaffoldParsesPlansAndRepeats(t *testing.T) {
	t.Parallel()
	s, git := setup(t)
	home, err := s.Host.HomeDir()
	must(t, err)
	dir := filepath.Join(home, "new-config")
	template := Template{Agents: []string{"claude", "codex"}, Instances: []string{"claude.main=~/.claude"}}
	exists, err := Scaffold(context.Background(), s.Host, git, dir, template)
	if err != nil || exists {
		t.Fatalf("scaffold: %t %v", exists, err)
	}
	m, err := manifest.Load(s.Host, filepath.Join(dir, "kitbag.toml"))
	if err != nil || len(m.Targets) != 2 {
		t.Fatalf("parse: %+v %v", m, err)
	}
	setAgentAdapters(t, &s, home)
	for _, agent := range []string{".claude", ".codex"} {
		err = s.Host.MkdirAll(filepath.Join(home, agent), 0o700)
		must(t, err)
	}
	assertScaffoldPlan(t, s, dir)
	exists, err = Scaffold(context.Background(), s.Host, git, dir, template)
	if err != nil || !exists {
		t.Fatalf("repeat: %t %v", exists, err)
	}
}

func TestScaffoldRefusesExistingFiles(t *testing.T) {
	t.Parallel()
	s, git := setup(t)
	home, err := s.Host.HomeDir()
	must(t, err)
	dir := filepath.Join(home, "scaffold")
	write(t, s.Host, filepath.Join(dir, "README.md"), "mine")
	_, err = Scaffold(context.Background(), s.Host, git, dir, Template{Agents: []string{"claude"}})
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("overwrite: %v", err)
	}
	if exists, existsErr := Exists(s.Host, filepath.Join(dir, "kitbag.toml")); exists || existsErr != nil {
		t.Fatalf("partial write: %v", existsErr)
	}
}

func TestTemplateRejectsInvalidSelections(t *testing.T) {
	t.Parallel()
	for _, template := range []Template{
		{Agents: []string{"unknown"}},
		{Agents: []string{"claude", "claude"}},
		{Agents: []string{"claude"}, Instances: []string{"codex.main=~/.codex"}},
		{Agents: []string{"claude"}, Instances: []string{"claude=~/.claude"}},
		{Agents: []string{"claude"}, Instances: []string{"claude.main=~/../outside"}},
		{Agents: []string{"claude"}, Instances: []string{"claude.main=~/.claude", "claude.main=~/.other"}},
	} {
		if _, err := template.Manifest(); err == nil {
			t.Fatalf("accepted %+v", template)
		}
	}
}

func setAgentAdapters(t *testing.T, s *Service, home string) {
	t.Helper()
	s.Adapters = func(_ *manifest.Manifest, _ *[]string) ([]target.Adapter, error) {
		claudeAdapter, adapterErr := claude.New(home, filepath.Join(home, ".claude"))
		if adapterErr != nil {
			return nil, adapterErr
		}
		codexAdapter, adapterErr := codex.New(s.Host)
		return []target.Adapter{claudeAdapter, codexAdapter}, adapterErr
	}
}

func assertScaffoldPlan(t *testing.T, s Service, dir string) {
	t.Helper()
	inspection, err := s.Inspect(dir)
	if err != nil || len(inspection.Plan.Operations) != 2 {
		t.Fatalf("scaffold plan: %+v %v", inspection, err)
	}
	for _, op := range inspection.Plan.Operations {
		if op.Warning != "" {
			t.Fatalf("unclean plan: %+v", op)
		}
	}
}
