package plan_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type fakeAdapter []target.Target

func (a fakeAdapter) Declare(_ manifest.FileSystem, _ string, _ *manifest.Manifest) ([]target.Target, error) {
	return a, nil
}

func path(parts ...string) string {
	return filepath.Join(append([]string{string(filepath.Separator)}, parts...)...)
}

func owner() []state.Owner { return []state.Owner{{Primitive: "skills", AgentInstance: "fake:main"}} }

func declaration(name string) target.Target {
	return target.Target{Path: path("home", name), SourcePath: path("repo", name), Kind: target.Materialized, Owners: owner()}
}

func request(declarations ...target.Target) plan.Request {
	return plan.Request{Manifest: &manifest.Manifest{Schema: 1}, Repository: path("repo"), State: state.Database{SchemaVersion: state.CurrentVersion}, Adapters: []target.Adapter{fakeAdapter(declarations)}}
}

func build(t *testing.T, h platform.FS, req plan.Request) plan.Plan {
	t.Helper()
	got, err := plan.Build(h, req)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func write(t *testing.T, h platform.FS, name, content string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, h platform.FS, source, dest string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.Link(source, dest); err != nil {
		t.Fatal(err)
	}
}

func tracked(d target.Target, hash string, kind state.Kind) state.Target {
	return state.Target{Path: d.Path, SourcePath: d.SourcePath, SourceHash: hash, WrittenHash: hash, Kind: kind, Owners: d.Owners}
}

func assertKinds(t *testing.T, p plan.Plan, want ...plan.Kind) {
	t.Helper()
	if len(p.Operations) != len(want) {
		t.Fatalf("operations = %s; want %v", p.Text(), want)
	}
	for i, kind := range want {
		if p.Operations[i].Kind != kind {
			t.Fatalf("operations = %s; want %v", p.Text(), want)
		}
	}
}

func assertClass(t *testing.T, p plan.Plan, want plan.Classification) {
	t.Helper()
	if len(p.Observations) != 1 || p.Observations[0].Classification != want {
		t.Fatalf("observations = %v; want %s", p.Observations, want)
	}
}

func hashTree(t *testing.T, h *fake.Host, name string) string {
	t.Helper()
	hash, err := plan.HashTree(h, name)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
