package main

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func TestCLIAdaptersSharedSkills(t *testing.T) {
	t.Parallel()
	h := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/user"}))
	if err := h.MkdirAll("/repo/skills/alpha", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic("/repo/skills/alpha/SKILL.md", []byte("Alpha.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Schema: 1, Skills: &manifest.Skills{Dirs: []string{"skills"}}, Targets: map[string]manifest.Target{
		"codex": {Agent: "codex"}, "gemini": {Agent: "gemini"}, "opencode": {Agent: "opencode"},
	}}
	var warnings []string
	c := commandLine{host: h}
	adapters, err := c.adapters(m, &warnings)
	if err != nil {
		t.Fatal(err)
	}
	req := plan.Request{Manifest: m, Repository: "/repo", Adapters: adapters, State: state.Database{SchemaVersion: state.CurrentVersion}}
	declarations, err := plan.Declarations(h, req)
	if err != nil {
		t.Fatal(err)
	}
	checkSharedSkillOwners(t, declarations)
	// Different sources at the shared destination must remain a hard error.
	req.Adapters = append(req.Adapters, adapterFunc(func(manifest.FileSystem, string, *manifest.Manifest) ([]target.Target, error) {
		return []target.Target{{Path: "/home/user/.agents/skills/alpha", SourcePath: "/repo/other/alpha", Kind: target.Materialized, Owners: []state.Owner{{Primitive: "skills", AgentInstance: "other/default"}}}}, nil
	}))
	if _, err = plan.Declarations(h, req); err == nil || !strings.Contains(err.Error(), "conflicting declarations") {
		t.Fatalf("source conflict accepted: %v", err)
	}
}

func checkSharedSkillOwners(t *testing.T, declarations []target.Target) {
	t.Helper()
	if len(declarations) != 2 {
		t.Fatalf("declarations: %+v", declarations)
	}
	shared := declarations[0]
	if shared.Path != "/home/user/.agents/skills/alpha" || len(shared.Owners) != 2 {
		t.Fatalf("shared owners: %+v", shared)
	}
	if shared.Owners[0].AgentInstance != "codex:codex/default" || shared.Owners[1].AgentInstance != "gemini:gemini/default" {
		t.Fatalf("wrong owners: %+v", shared.Owners)
	}
	native := declarations[1]
	if native.Path != "/home/user/.config/opencode/skills/alpha" || native.Mode != "copy" {
		t.Fatalf("OpenCode skill: %+v", native)
	}
}
