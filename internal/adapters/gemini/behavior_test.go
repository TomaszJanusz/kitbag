package gemini_test

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/gemini"
	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestOverrideAndMarker(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	text := strings.Replace(everything, "source = \"global/AGENTS.md\"", "source = \"global/AGENTS.md\"\nper_agent.gemini = \"agents/override.md\"", 1)
	r := f.plan(t, text)
	d, ok := find(r.Targets, agentHome+"/GEMINI.md")
	if !ok || !instructions.Managed(d.Content) || !strings.Contains(string(d.Content), "Agent-specific instructions") || strings.Contains(string(d.Content), "Shared") {
		t.Fatalf("override or marker missing: %+v", d)
	}
	declared, err := mustAdapter(t, f).Declare(f.host, repository, f.manifest(t, text))
	if err != nil || !reflect.DeepEqual(declared, r.Targets) {
		t.Fatalf("Declare differs: %v", err)
	}
}

func mustAdapter(t *testing.T, f fixture) gemini.Adapter {
	t.Helper()
	a, err := gemini.New(f.host)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestWarningsAndDisabled(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.selected = "absent"
	r := f.plan(t, everything)
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "not found") {
		t.Fatalf("warnings: %v", r.Warnings)
	}
	for _, edit := range []string{"enabled = false", "primitives.instructions.enabled = false\nprimitives.skills.enabled = false\nprimitives.commands.enabled = false"} {
		r = f.plan(t, everything+edit+"\n")
		if len(r.Targets) != 0 {
			t.Fatalf("disabled targets: %+v", r)
		}
	}
	if _, err := (gemini.Adapter{}).Declare(f.host, repository, nil); err == nil {
		t.Fatal("zero adapter accepted")
	}
	if _, err := mustAdapter(t, f).Plan(f.host, "relative", nil); err == nil {
		t.Fatal("relative repository accepted")
	}
}

func TestMissingInstanceAndSettings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	text := everything + "settings = \"gemini/config.toml\"\n[targets.gemini.instances.missing]\ndir = \"~/.missing\"\n"
	p := f.build(t, text)
	for _, op := range p.Operations {
		if strings.Contains(op.Path, "/.missing/") && (op.Kind != plan.Skip || !strings.Contains(op.Warning, "instance directory is missing")) {
			t.Fatalf("missing instance: %+v", op)
		}
	}
	r := f.plan(t, everything+"settings = \"gemini/config.toml\"\n")
	d, ok := find(r.Targets, agentHome+"/settings.json")
	if !ok || d.Warning == "" || d.Merger != nil || d.SourcePath != "" {
		t.Fatalf("settings written: %+v", d)
	}
}

func TestNativeCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.write(t, repository+"/gemini-commands/build.toml", "prompt = \"Override\"\n")
	r := f.plan(t, everything)
	for _, d := range r.Targets {
		if !strings.Contains(d.Path, "/commands/") {
			continue
		}
		native := strings.HasSuffix(d.Path, ".toml")
		if native != (d.Warning == "") || d.Mode != "copy" {
			t.Fatalf("format handling: %+v", d)
		}
	}
	d, ok := find(r.Targets, agentHome+"/commands/build.toml")
	if !ok || d.SourcePath != repository+"/gemini-commands/build.toml" || d.Kind != target.Materialized {
		t.Fatalf("command override: %+v", d)
	}
}

func TestHomeResolution(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ value, want string }{
		{"", agentHome}, {"/srv/custom", "/srv/custom/.gemini"}, {"/srv/custom/", "/srv/custom/.gemini"},
	} {
		t.Run(c.value, func(t *testing.T) {
			t.Parallel()
			r := newFixture(t, map[string]string{"GEMINI_CLI_HOME": c.value}).plan(t, everything)
			if _, ok := find(r.Targets, c.want+"/GEMINI.md"); !ok {
				t.Fatalf("wrong home: %s", describe(r))
			}
		})
	}
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "GEMINI_CLI_HOME": "relative"}))
	_, err := gemini.New(h)
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("relative home: %v", err)
	}
}

func TestUnsupportedPrimitives(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	text := withHost + `[hooks]
dir = "agents"
[rules]
dir = "agents"
[mcp.servers.example]
command = "example"
[targets.gemini]
agent = "gemini"
`
	r := f.plan(t, text)
	if len(r.Targets) != 1 || len(r.Warnings) != 3 {
		t.Fatalf("unsupported primitive written: %s", describe(r))
	}
}

func TestInstructionRender(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	r, err := f.planErr(t, everything+"primitives.instructions.render = \"import\"\n")
	if err != nil || !strings.Contains(string(r.Targets[0].Content), "@/repo/global/AGENTS.md") {
		t.Fatalf("import: %+v %v", r, err)
	}
}
