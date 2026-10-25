package claude_test

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/claude"
	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func write(t *testing.T, h *fake.Host, path, data string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func adapter(t *testing.T, directory string) claude.Adapter {
	t.Helper()
	a, err := claude.New("/home/test", directory)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func fixture(t *testing.T) (*fake.Host, *manifest.Manifest) {
	t.Helper()
	h := fake.New()
	for path, data := range map[string]string{
		"global/AGENTS.md": "Shared instructions.\n", "skills/tool/SKILL.md": "Tool skill.",
		"skills/readme.txt": "not a skill", "shared/build.md": "shared", "claude/build.md": "override",
		"agents/reviewer.md": "Review changes.", "hooks/check.sh": "#!/bin/sh\nexit 0\n",
		"rules/style.md": "Use clear names.", "settings.json": `{"theme":"dark"}`,
	} {
		write(t, h, "/repo/"+path, data)
	}
	return h, &manifest.Manifest{
		Schema: 1, Instructions: &manifest.Instructions{Source: "global/AGENTS.md"},
		Skills: &manifest.Skills{Dirs: []string{"skills"}}, Commands: &manifest.Collection{Dir: "shared", PerAgent: map[string]string{"claude": "claude"}},
		Agents: &manifest.Collection{Dir: "agents"}, Hooks: &manifest.Collection{Dir: "hooks"}, Rules: &manifest.Collection{Dir: "rules"},
		Targets: map[string]manifest.Target{"work": {Agent: "claude", Settings: "settings.json"}},
	}
}

func declarations(t *testing.T, h *fake.Host, m *manifest.Manifest) []target.Target {
	t.Helper()
	ds, err := adapter(t, "").Declare(h, "/repo", m)
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func find(t *testing.T, ds []target.Target, path string) target.Target {
	t.Helper()
	for _, d := range ds {
		if d.Path == path {
			return d
		}
	}
	t.Fatalf("missing declaration %s", path)
	return target.Target{}
}

func TestDefaultPrimitives(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	ds := declarations(t, h, m)
	if len(ds) != 7 {
		t.Fatalf("declarations: %+v", ds)
	}
	assertMaterialized(t, ds)
	if d := find(t, ds, "/home/test/.claude/commands/build.md"); d.SourcePath != "/repo/claude/build.md" {
		t.Fatal(d.SourcePath)
	}
	d := find(t, ds, "/home/test/.claude/CLAUDE.md")
	if string(d.Content) != instructions.Marker+"\n\n@/repo/global/AGENTS.md\n" {
		t.Fatalf("import: %s", d.Content)
	}
	d = find(t, ds, "/home/test/.claude/settings.json")
	if d.Kind != target.Merged || d.Merger == nil || d.SourcePath != "/repo/settings.json" {
		t.Fatalf("settings: %+v", d)
	}
	again := declarations(t, h, m)
	if !reflect.DeepEqual(ds, again) {
		t.Fatal("declarations are nondeterministic")
	}
}

func TestInstancesAndOverrides(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	off := false
	config := m.Targets["work"]
	config.Mode = "link"
	config.Primitives = map[string]manifest.Primitive{"instructions": {Render: "concat"}, "commands": {Enabled: &off}, "skills": {Mode: "copy"}}
	config.Instances = map[string]manifest.Instance{"second": {
		Dir: "~/.claude_2", Settings: &off,
		Roots:      map[string]string{"skills": "~/custom/skills", "instructions": "~/custom/CLAUDE.md"},
		Primitives: map[string]manifest.Primitive{"instructions": {Render: "import"}, "skills": {Mode: "link"}},
	}}
	m.Targets["work"] = config
	ds := declarations(t, h, m)
	if len(ds) != 6 {
		t.Fatalf("declarations: %+v", ds)
	}
	for _, d := range ds {
		if d.InstanceDir != "/home/test/.claude_2" {
			t.Fatalf("missing instance guard: %+v", d)
		}
	}
	if d := find(t, ds, "/home/test/custom/skills/tool"); d.Mode != "link" {
		t.Fatal(d.Mode)
	}
	if d := find(t, ds, "/home/test/custom/CLAUDE.md"); !strings.Contains(string(d.Content), "@/repo/") {
		t.Fatal(string(d.Content))
	}
	if d := find(t, ds, "/home/test/.claude_2/settings.json"); !d.Preserve {
		t.Fatal("settings=off ignored")
	}
}

func TestRelocatedDefault(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	ds, err := adapter(t, "/home/test/relocated").Declare(h, "/repo", m)
	if err != nil {
		t.Fatal(err)
	}
	if d := find(t, ds, "/home/test/relocated/settings.json"); d.InstanceDir != "" {
		t.Fatal("default relocation incorrectly requires pre-existing directory")
	}
	for _, paths := range [][2]string{{"relative", ""}, {"/home/test", "relative"}, {"/home/test/..", ""}} {
		if _, err = claude.New(paths[0], paths[1]); err == nil {
			t.Fatalf("accepted unsafe paths %v", paths)
		}
	}
}

func TestReservedSkillAndNativeFormats(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	write(t, h, "/repo/shared/unsupported.toml", "prompt = 'hello'")
	d := find(t, declarations(t, h, m), "/home/test/.claude/commands/unsupported.toml")
	if d.Warning == "" {
		t.Fatal("foreign format accepted")
	}
	write(t, h, "/repo/skills/synced/SKILL.md", "reserved")
	if _, err := adapter(t, "").Declare(h, "/repo", m); err == nil {
		t.Fatal("accepted reserved skill")
	}
}

func TestDisabledTargetsAndInstances(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	off := false
	m.Targets["other"] = manifest.Target{Agent: "codex"}
	m.Targets["disabled"] = manifest.Target{Agent: "claude", Enabled: &off}
	config := m.Targets["work"]
	config.Instances = map[string]manifest.Instance{"disabled": {Dir: "~/.disabled", Enabled: &off}}
	m.Targets["work"] = config
	if ds := declarations(t, h, m); len(ds) != 0 {
		t.Fatalf("disabled declarations: %+v", ds)
	}
}

func build(t *testing.T, h *fake.Host, m *manifest.Manifest, previous []state.Target) plan.Plan {
	t.Helper()
	p, err := plan.Build(h, plan.Request{Repository: "/repo", Manifest: m, Adapters: []target.Adapter{adapter(t, "")}, State: state.Database{SchemaVersion: state.CurrentVersion, Targets: previous}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSettingsOffProtectsPriorOwnership(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	path := "/home/test/.claude/settings.json"
	original := `{"local":true}`
	write(t, h, path, original)
	config := m.Targets["work"]
	off := false
	config.Instances = map[string]manifest.Instance{"main": {Dir: "~/.claude", Settings: &off}}
	m.Targets["work"] = config
	previous := []state.Target{{Path: path, Kind: state.Merged, WrittenHash: plan.HashBytes([]byte(original)), Owners: []state.Owner{{Primitive: "settings", AgentInstance: "claude:/home/test/.claude"}}}}
	for _, op := range build(t, h, m, previous).Operations {
		if op.Path == path {
			t.Fatalf("settings=off produced %s", op.Kind)
		}
	}
}

func TestUnsafeSettingsAndMissingInstance(t *testing.T) {
	t.Parallel()
	for _, data := range []string{"{bad", `["local"]`, `null`} {
		h, m := fixture(t)
		write(t, h, "/home/test/.claude/settings.json", data)
		assertSkipped(t, build(t, h, m, nil), "/home/test/.claude/settings.json", "settings merge rejected")
	}
	h, m := fixture(t)
	config := m.Targets["work"]
	config.Instances = map[string]manifest.Instance{"missing": {Dir: "~/.missing"}}
	m.Targets["work"] = config
	for _, op := range build(t, h, m, nil).Operations {
		if op.Kind != plan.Skip || !strings.Contains(op.Warning, "instance directory is missing") {
			t.Fatalf("missing instance: %+v", op)
		}
	}
}

func assertSkipped(t *testing.T, p plan.Plan, path, warning string) {
	t.Helper()
	for _, op := range p.Operations {
		if op.Path == path {
			if op.Kind != plan.Skip || !strings.Contains(op.Warning, warning) {
				t.Fatalf("unsafe target: %+v", op)
			}
			return
		}
	}
	t.Fatalf("no warning for %s", path)
}

func TestSyncedUntouched(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	path := "/home/test/.claude/skills/synced"
	if err := h.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.Link("/repo/removed", path); err != nil {
		t.Fatal(err)
	}
	for _, op := range build(t, h, m, nil).Operations {
		if strings.Contains(op.Path, "/skills/synced") {
			t.Fatalf("touched reserved skill: %+v", op)
		}
	}
	info, err := h.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("reserved link changed: %v", err)
	}
}

func assertMaterialized(t *testing.T, ds []target.Target) {
	t.Helper()
	for _, suffix := range []string{"skills/tool", "commands/build.md", "agents/reviewer.md", "hooks/check.sh", "rules/style.md"} {
		d := find(t, ds, "/home/test/.claude/"+suffix)
		if d.Kind != target.Materialized || d.InstanceDir != "" || d.Backup != target.Once {
			t.Fatalf("materialization: %+v", d)
		}
	}
}

func TestReservedDestinationRoots(t *testing.T) {
	t.Parallel()
	for _, primitive := range []string{"instructions", "skills", "commands", "agents", "hooks", "rules"} {
		h, m := fixture(t)
		config := m.Targets["work"]
		config.Instances = map[string]manifest.Instance{"main": {Dir: "~/.claude", Roots: map[string]string{primitive: "~/.claude/skills/synced"}}}
		m.Targets["work"] = config
		if _, err := adapter(t, "").Declare(h, "/repo", m); err == nil || !strings.Contains(err.Error(), "reserved skills/synced") {
			t.Fatalf("%s root accepted: %v", primitive, err)
		}
	}
}

func TestMissingHostWarningDoesNotCreateATarget(t *testing.T) {
	t.Parallel()
	h, m := fixture(t)
	m.Instructions.Hosts = "hosts/missing.md"
	result, err := adapter(t, "").Plan(h, "/repo", m)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 7 || len(result.Warnings) != 1 {
		t.Fatalf("warning changed targets: %+v", result)
	}
	d := find(t, result.Targets, "/home/test/.claude/CLAUDE.md")
	if d.Warning != "" || !strings.Contains(string(d.Content), "@/repo/global/AGENTS.md") {
		t.Fatalf("missing host suppressed imports: %+v", d)
	}
}
