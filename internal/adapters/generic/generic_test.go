package generic_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/generic"
	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func path(parts ...string) string {
	return filepath.Join(append([]string{string(filepath.Separator)}, parts...)...)
}

func repo(parts ...string) string { return path(append([]string{"repo"}, parts...)...) }
func home(parts ...string) string { return path(append([]string{"home"}, parts...)...) }

const base = `schema = 1

[instructions]
source = "AGENTS.md"
hosts = "hosts/{host}.md"

[skills]
dirs = ["skills", "vendor"]

[commands]
dir = "commands"
per_agent = { generic = "commands-generic", codex = "commands-codex" }

[agents]
dir = "agents"
`

func parse(t *testing.T, extra string) *manifest.Manifest {
	t.Helper()
	return resolve(t, base+extra)
}

// resolve parses and host-resolves a manifest for host "test".
func resolve(t *testing.T, data string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse("kitbag.toml", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": "test"})), "")
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func fixture(t *testing.T) *fake.Host {
	t.Helper()
	h := fake.New()
	for name, content := range map[string]string{
		"AGENTS.md":                    "Shared.\n",
		"hosts/test.md":                "Host.\n",
		"skills/review/SKILL.md":       "Review.\n",
		"skills/notes/README.md":       "Not a skill.\n",
		"skills/loose.md":              "Not a directory.\n",
		"vendor/browser/SKILL.md":      "Browser.\n",
		"commands/deploy.md":           "Shared deploy.\n",
		"commands/test.md":             "Shared test.\n",
		"commands-generic/deploy.md":   "Generic deploy.\n",
		"commands-codex/codex-only.md": "Codex.\n",
		"agents/reviewer.md":           "Reviewer.\n",
	} {
		name = repo(filepath.FromSlash(name))
		if err := h.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := h.WriteAtomic(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func declare(t *testing.T, h *fake.Host, m *manifest.Manifest) []target.Target {
	t.Helper()
	got, err := generic.New(home()).Declare(h, repo(), m)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// summary is the comparable part of a declaration.
type summary struct {
	Path, Source, Mode, Instance, Warning string
	Kind                                  target.Kind
	Backup                                target.BackupPolicy
	Owners                                []state.Owner
}

func summarize(targets []target.Target) []summary {
	out := make([]summary, 0, len(targets))
	for _, d := range targets {
		out = append(out, summary{d.Path, d.SourcePath, d.Mode, d.InstanceDir, d.Warning, d.Kind, d.Backup, d.Owners})
	}
	return out
}

func owners(primitive string) []state.Owner {
	return []state.Owner{{Primitive: primitive, AgentInstance: "amp/main"}}
}

func TestDeclareRootsAndPolicy(t *testing.T) {
	t.Parallel()
	m := parse(t, `
[targets.amp]
agent = "generic"
mode = "link"
primitives = { agents = { enabled = false } }

[targets.amp.instances.main]
dir = "~/.amp"
roots = { instructions = "~/.amp/AGENTS.md", skills = "~/.amp/skills", commands = "~/.amp/commands", agents = "~/.amp/agents" }
primitives = { skills = { mode = "copy" } }

[targets.amp.instances.off]
dir = "~/.amp-off"
enabled = false
roots = { skills = "~/.amp-off/skills" }

[targets.disabled]
agent = "generic"
enabled = false

[targets.disabled.instances.main]
dir = "~/.disabled"
roots = { skills = "~/.disabled/skills" }

[targets.codex]
agent = "codex"
`)
	got := declare(t, fixture(t), m)
	dir := home(".amp")
	want := []summary{
		{Path: home(".amp", "commands", "deploy.md"), Source: repo("commands-generic", "deploy.md"), Mode: "link", Instance: dir, Kind: target.Materialized, Owners: owners("commands")},
		{Path: home(".amp", "commands", "test.md"), Source: repo("commands", "test.md"), Mode: "link", Instance: dir, Kind: target.Materialized, Owners: owners("commands")},
		{Path: home(".amp", "AGENTS.md"), Instance: dir, Kind: target.Generated, Backup: target.Once, Owners: owners("instructions")},
		{Path: home(".amp", "skills", "review"), Source: repo("skills", "review"), Mode: "copy", Instance: dir, Kind: target.Materialized, Owners: owners("skills")},
		{Path: home(".amp", "skills", "browser"), Source: repo("vendor", "browser"), Mode: "copy", Instance: dir, Kind: target.Materialized, Owners: owners("skills")},
	}
	if s := summarize(got); !slices.EqualFunc(s, want, equalSummary) {
		t.Fatalf("declarations:\ngot  %+v\nwant %+v", s, want)
	}
	wantContent := instructions.Marker + "\n\nShared.\n\nHost.\n"
	if string(got[2].Content) != wantContent {
		t.Fatalf("instructions = %q, want %q", got[2].Content, wantContent)
	}
}

func equalSummary(a, b summary) bool {
	return a.Path == b.Path && a.Source == b.Source && a.Mode == b.Mode && a.Instance == b.Instance &&
		a.Warning == b.Warning && a.Kind == b.Kind && a.Backup == b.Backup && slices.Equal(a.Owners, b.Owners)
}

func TestInstructionsRender(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, primitives, wantErr string
	}{
		{name: "default concat"},
		{name: "explicit concat", primitives: `primitives = { instructions = { render = "concat" } }`},
		{name: "import has no generic syntax", primitives: `primitives = { instructions = { render = "import" } }`, wantErr: `render "import" is not supported`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := parse(t, "[targets.x]\nagent = \"generic\"\n[targets.x.instances.main]\ndir = \"~/.x\"\nroots = { instructions = \"~/.x/AGENTS.md\" }\n"+tt.primitives+"\n")
			got, err := generic.New(home()).Declare(fixture(t), repo(), m)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || len(got) != 1 || got[0].Kind != target.Generated {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestInstancePrimitiveOverridesTarget(t *testing.T) {
	t.Parallel()
	m := parse(t, `
[targets.x]
agent = "generic"
primitives = { skills = { enabled = false } }

[targets.x.instances.main]
dir = "~/.x"
roots = { skills = "~/.x/skills" }
primitives = { skills = { enabled = true } }
`)
	if got := declare(t, fixture(t), m); len(got) != 2 {
		t.Fatalf("instance enabled = true must win over the target: %+v", summarize(got))
	}
}

func TestMCPRootIsUnsupported(t *testing.T) {
	t.Parallel()
	m := parse(t, `
[mcp.servers.docs]
command = "docs-mcp"

[targets.x]
agent = "generic"

[targets.x.instances.main]
dir = "~/.x"
roots = { mcp = "~/.x/mcp.json" }
`)
	got := declare(t, fixture(t), m)
	if len(got) != 1 || got[0].Warning == "" || got[0].Path != home(".x", "mcp.json") {
		t.Fatalf("want one unsupported MCP declaration, got %+v", summarize(got))
	}
}

func TestUnmanagedPrimitivesAndMissingSources(t *testing.T) {
	t.Parallel()
	m := resolve(t, `schema = 1

[skills]
dirs = ["gone"]

[targets.x]
agent = "generic"

[targets.x.instances.main]
dir = "~/.x"
roots = { instructions = "~/.x/AGENTS.md", skills = "~/.x/skills", commands = "~/.x/commands", rules = "~/.x/rules" }
`)
	if got := declare(t, fixture(t), m); len(got) != 0 {
		t.Fatalf("unmanaged primitives and missing sources declare nothing, got %+v", summarize(got))
	}
}

func TestRelativeHomeRejected(t *testing.T) {
	t.Parallel()
	if _, err := generic.New("home").Declare(fixture(t), repo(), parse(t, "")); err == nil {
		t.Fatal("relative home accepted")
	}
}
