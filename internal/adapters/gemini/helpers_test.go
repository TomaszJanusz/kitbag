package gemini_test

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/gemini"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Flags are registered locally, with no package globals. Update reviewed goldens:
//
//	mise exec -- go test ./internal/adapters/gemini -run '^TestGolden$' -args -update
func TestMain(m *testing.M) {
	flag.Bool("update", false, "rewrite reviewed gemini declaration goldens")
	flag.Parse()
	os.Exit(m.Run())
}

const (
	home       = "/home/user"
	repository = "/repo"
	// The shared instruction fixtures, so the adapter is compared against the
	// same agent-config-sync output as internal/adapters/instructions.
	sharedFixtures = "../instructions/testdata"
)

// fixture is an in-memory host with HOME, the shared instruction fixture
// repository at /repo plus generic skills and commands, and a manifest.
type fixture struct {
	host *fake.Host
	// selected is the host name; hosts/absent.md does not exist.
	selected string
}

func newFixture(t *testing.T, env map[string]string) fixture {
	t.Helper()
	vars := map[string]string{"HOME": home}
	for key, value := range env {
		vars[key] = value
	}
	f := fixture{host: fake.New(fake.WithEnv(vars)), selected: "example"}
	copyTree(t, f.host, filepath.Join(sharedFixtures, "repo"), repository)
	for name, content := range map[string]string{
		"skills/alpha/SKILL.md":     "---\nname: alpha\n---\nAlpha skill.\n",
		"skills/beta/SKILL.md":      "---\nname: beta\n---\nBeta skill.\n",
		"skills/beta/notes.md":      "Beta notes.\n",
		"skills/SOURCES.md":         "Not a skill.\n",
		"skills/empty/README.md":    "No SKILL.md, not a skill.\n",
		"vendor/gamma/SKILL.md":     "---\nname: gamma\n---\nGamma skill.\n",
		"commands/review.md":        "---\ndescription: Review a change\n---\nReview $1.\n",
		"commands/shared.md":        "Shared command.\n",
		"commands/build.toml":       "prompt = \"Gemini format\"\n",
		"commands/nested/inner.md":  "A directory entry.\n",
		"gemini-commands/shared.md": "gemini-specific command.\n",
		"gemini/config.toml":        "model = \"example\"\n",
		"agents/reviewer.toml":      "name = \"reviewer\"\n",
	} {
		f.write(t, filepath.Join(repository, filepath.FromSlash(name)), content)
	}
	return f
}

func (f fixture) write(t *testing.T, name, content string) {
	t.Helper()
	if err := f.host.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.host.WriteAtomic(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// manifest loads, validates and host-resolves kitbag.toml for f.selected.
func (f fixture) manifest(t *testing.T, text string) *manifest.Manifest {
	t.Helper()
	f.write(t, filepath.Join(repository, "kitbag.toml"), text)
	m, err := manifest.Load(f.host, filepath.Join(repository, "kitbag.toml"))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := m.ResolveHost(f.host, f.selected)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func (f fixture) plan(t *testing.T, text string) gemini.Result {
	t.Helper()
	result, err := f.planErr(t, text)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f fixture) planErr(t *testing.T, text string) (gemini.Result, error) {
	t.Helper()
	adapter, err := gemini.New(f.host)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.Plan(f.host, repository, f.manifest(t, text))
}

// build runs the real planner over the adapter's declarations.
func (f fixture) build(t *testing.T, text string) plan.Plan {
	t.Helper()
	adapter, err := gemini.New(f.host)
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Build(f.host, plan.Request{
		Manifest: f.manifest(t, text), Repository: repository,
		Adapters: []target.Adapter{adapter}, State: state.Database{SchemaVersion: state.CurrentVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func find(targets []target.Target, path string) (target.Target, bool) {
	for _, t := range targets {
		if t.Path == path {
			return t, true
		}
	}
	return target.Target{}, false
}

func describe(r gemini.Result) string {
	var lines []string
	for _, d := range r.Targets {
		lines = append(lines, d.Path)
		for _, field := range [][2]string{
			{"kind", string(d.Kind)},
			{"mode", d.Mode},
			{"source", d.SourcePath},
			{"instance", d.InstanceDir},
			{"backup", string(d.Backup)},
			{"warning", d.Warning},
		} {
			if field[1] != "" {
				lines = append(lines, "  "+field[0]+": "+field[1])
			}
		}
		for _, owner := range d.Owners {
			lines = append(lines, "  owner: "+owner.Primitive+" "+owner.AgentInstance)
		}
		if d.Content != nil {
			lines = append(lines, "  content: "+strings.ReplaceAll(strings.TrimSuffix(string(d.Content), "\n"), "\n", "\n  | "))
		}
	}
	for _, warning := range r.Warnings {
		lines = append(lines, "non-blocking warning: "+warning)
	}
	return strings.Join(lines, "\n") + "\n"
}

func copyTree(t *testing.T, h *fake.Host, from, to string) {
	t.Helper()
	root := os.DirFS(from)
	err := fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		name := filepath.Join(to, filepath.FromSlash(path))
		if err = h.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			return err
		}
		return h.WriteAtomic(name, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	dir := filepath.Join("testdata", "golden")
	if flag.Lookup("update").Value.String() == "true" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(os.DirFS(dir), name)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}
