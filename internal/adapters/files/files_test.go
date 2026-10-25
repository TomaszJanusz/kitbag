package files_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/files"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func path(parts ...string) string {
	return filepath.Join(append([]string{string(filepath.Separator)}, parts...)...)
}

func resolve(t *testing.T, data string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse("kitbag.toml", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := m.ResolveHost(fake.New(fake.WithEnv(map[string]string{"KITBAG_HOST": "mac"})), "")
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

const fixture = `schema = 1
mode = "link"

[[files]]
src = "agent-browser/config.json"
dest = "~/.agent-browser/config.json"

[[files]]
src = "tools"
dest = "~/.config/tools"
mode = "copy"

[[files]]
src = "runner.conf"
dest = "~/.runner.conf"
hosts = ["runner02"]
`

func TestDeclare(t *testing.T) {
	t.Parallel()
	got, err := files.New(path("home")).Declare(fake.New(), path("repo"), resolve(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	owners := []state.Owner{{Primitive: "files", AgentInstance: "files"}}
	want := []target.Target{
		{Path: path("home", ".agent-browser", "config.json"), SourcePath: path("repo", "agent-browser", "config.json"), Kind: target.Materialized, Backup: target.Once, Mode: "link", Owners: owners},
		{Path: path("home", ".config", "tools"), SourcePath: path("repo", "tools"), Kind: target.Materialized, Backup: target.Once, Mode: "copy", Owners: owners},
	}
	if !slices.EqualFunc(got, want, func(a, b target.Target) bool {
		return a.Path == b.Path && a.SourcePath == b.SourcePath && a.Kind == b.Kind && a.Backup == b.Backup &&
			a.Mode == b.Mode && a.InstanceDir == "" && slices.Equal(a.Owners, b.Owners)
	}) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

// TestPreexistingFileIsBackedUpOnce checks the planner's decision for the
// declaration: an unmanaged regular file is backed up before linking, and a
// backup that already exists blocks a second one.
func TestPreexistingFileIsBackedUpOnce(t *testing.T) {
	t.Parallel()
	h := fake.New()
	dest := path("home", ".agent-browser", "config.json")
	write(t, h, path("repo", "agent-browser", "config.json"), "{}\n")
	write(t, h, dest, "{\"local\": true}\n")
	req := plan.Request{
		Manifest:   resolve(t, fixture),
		Repository: path("repo"),
		State:      state.Database{SchemaVersion: state.CurrentVersion},
		Adapters:   []target.Adapter{files.New(path("home"))},
	}
	if kinds := operationKinds(t, h, req); !slices.Equal(kinds, []plan.Kind{plan.Backup, plan.Materialize}) {
		t.Fatalf("operations = %v, want backup then materialize", kinds)
	}
	write(t, h, dest+".before-kitbag.20260101T000000.000000000Z-0", "{\"older\": true}\n")
	if kinds := operationKinds(t, h, req); !slices.Equal(kinds, []plan.Kind{plan.Skip}) {
		t.Fatalf("operations = %v, want skip once a backup exists", kinds)
	}
}

func operationKinds(t *testing.T, h *fake.Host, req plan.Request) []plan.Kind {
	t.Helper()
	p, err := plan.Build(h, req)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]plan.Kind, 0, len(p.Operations))
	for _, op := range p.Operations {
		kinds = append(kinds, op.Kind)
	}
	return kinds
}

func write(t *testing.T, h *fake.Host, name, content string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRelativeHomeRejected(t *testing.T) {
	t.Parallel()
	if _, err := files.New("home").Declare(fake.New(), path("repo"), resolve(t, fixture)); err == nil {
		t.Fatal("relative home accepted")
	}
}
