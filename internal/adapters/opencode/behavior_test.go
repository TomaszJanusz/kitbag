package opencode_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/adapters/opencode"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestOverrideAndMarker(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	text := strings.Replace(everything, "source = \"global/AGENTS.md\"", "source = \"global/AGENTS.md\"\nper_agent.opencode = \"agents/override.md\"", 1)
	r := f.plan(t, text)
	d, ok := find(r.Targets, agentHome+"/AGENTS.md")
	if !ok || !instructions.Managed(d.Content) || !strings.Contains(string(d.Content), "Agent-specific instructions") || strings.Contains(string(d.Content), "Shared") {
		t.Fatalf("override or marker missing: %+v", d)
	}
	declared, err := mustAdapter(t, f).Declare(f.host, repository, f.manifest(t, text))
	if err != nil || !reflect.DeepEqual(declared, r.Targets) {
		t.Fatalf("Declare differs: %v", err)
	}
}

func mustAdapter(t *testing.T, f fixture) opencode.Adapter {
	t.Helper()
	a, err := opencode.New(f.host)
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
	if _, err := (opencode.Adapter{}).Declare(f.host, repository, nil); err == nil {
		t.Fatal("zero adapter accepted")
	}
	if _, err := mustAdapter(t, f).Plan(f.host, "relative", nil); err == nil {
		t.Fatal("relative repository accepted")
	}
}

func TestMissingInstanceAndSettings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	text := everything + "settings = \"opencode/config.toml\"\n[targets.opencode.instances.missing]\ndir = \"~/.missing\"\n"
	p := f.build(t, text)
	for _, op := range p.Operations {
		if strings.Contains(op.Path, "/.missing/") && (op.Kind != plan.Skip || !strings.Contains(op.Warning, "instance directory is missing")) {
			t.Fatalf("missing instance: %+v", op)
		}
	}
	r := f.plan(t, everything+"settings = \"opencode/config.toml\"\n")
	d, ok := find(r.Targets, agentHome+"/opencode.json")
	if !ok || d.Warning == "" || d.Merger != nil || d.SourcePath != "" {
		t.Fatalf("settings written: %+v", d)
	}
}

func TestNativeCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.write(t, repository+"/opencode-commands/build.toml", "prompt = \"Override\"\n")
	r := f.plan(t, everything)
	for _, d := range r.Targets {
		if !strings.Contains(d.Path, "/commands/") {
			continue
		}
		native := strings.HasSuffix(d.Path, ".md")
		if native != (d.Warning == "") || d.Mode != "copy" {
			t.Fatalf("format handling: %+v", d)
		}
	}
	d, ok := find(r.Targets, agentHome+"/commands/shared.md")
	if !ok || d.SourcePath != repository+"/opencode-commands/shared.md" || d.Kind != target.Materialized {
		t.Fatalf("command override: %+v", d)
	}
}

func TestHomeResolution(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ value, want string }{
		{"", agentHome}, {"/srv/custom", "/srv/custom/opencode"}, {"/srv/custom/", "/srv/custom/opencode"},
	} {
		t.Run(c.value, func(t *testing.T) {
			t.Parallel()
			r := newFixture(t, map[string]string{"XDG_CONFIG_HOME": c.value}).plan(t, everything)
			if _, ok := find(r.Targets, c.want+"/AGENTS.md"); !ok {
				t.Fatalf("wrong home: %s", describe(r))
			}
		})
	}
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "XDG_CONFIG_HOME": "relative"}))
	_, err := opencode.New(h)
	if err != nil {
		t.Fatal(err)
	}
	r := newFixture(t, map[string]string{"XDG_CONFIG_HOME": "relative", "OPENCODE_CONFIG_DIR": "/ignored"}).plan(t, everything)
	if _, ok := find(r.Targets, agentHome+"/AGENTS.md"); !ok {
		t.Fatalf("relative XDG or extra config changed home: %s", describe(r))
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
[targets.opencode]
agent = "opencode"
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
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("import accepted: %+v %v", r, err)
	}
}
