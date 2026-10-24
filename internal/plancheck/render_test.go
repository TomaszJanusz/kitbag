// Package plancheck_test keeps golden fixture I/O outside core, just as
// schemacheck does for manifest diagnostics. The planner only receives fake FS.
package plancheck_test

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Flags are registered locally, with no package globals.
func TestMain(m *testing.M) {
	flag.Bool("update", false, "rewrite reviewed plan rendering goldens")
	flag.Parse()
	os.Exit(m.Run())
}

func TestPlanRenderings(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"empty", "mixed"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			req := goldenRequest(t, h, name)
			p, err := plan.Build(h, req)
			if err != nil {
				t.Fatal(err)
			}
			data, err := p.JSON()
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name+".json", string(data))
			golden(t, name+".txt", p.Text())
			if strings.Contains(string(data), "secret content") {
				t.Fatal("rendering leaked contents")
			}
		})
	}
}

type declarations []target.Target

func (d declarations) Declare(_ manifest.FileSystem, _ string, _ *manifest.Manifest) ([]target.Target, error) {
	return d, nil
}

type testMerger struct{}

func (testMerger) Merge(_, fragment []byte) ([]byte, error) {
	return append([]byte(nil), fragment...), nil
}

func goldenRequest(t *testing.T, h *fake.Host, name string) plan.Request {
	t.Helper()
	req := plan.Request{Manifest: &manifest.Manifest{Schema: 1}, Repository: "/repo", State: state.Database{SchemaVersion: state.CurrentVersion}}
	if name == "empty" {
		return req
	}
	owners := []state.Owner{{Primitive: "skills", AgentInstance: "fake:main"}}
	writeFixture(t, h, "/repo/skill", "skill")
	writeFixture(t, h, "/home/generated", "original")
	writeFixture(t, h, "/home/drift", "local")
	writeFixture(t, h, "/home/refresh", "same")
	req.Adapters = []target.Adapter{declarations{
		{Path: "/home/skill", SourcePath: "/repo/skill", Kind: target.Materialized, Owners: owners},
		{Path: "/home/generated", Kind: target.Generated, Content: []byte("secret content"), Owners: owners, Backup: target.Once},
		{Path: "/home/drift", Kind: target.Generated, Content: []byte("old"), Owners: owners},
		{Path: "/home/settings", Kind: target.Merged, Content: []byte("{}"), Merger: testMerger{}, Owners: owners},
		{Path: "/home/refresh", Kind: target.Generated, Content: []byte("same"), Owners: owners},
	}}
	req.State.Targets = []state.Target{
		{Path: "/home/stale", Kind: state.Generated, WrittenHash: plan.HashBytes(nil), Owners: owners},
		{Path: "/home/drift", Kind: state.Generated, WrittenHash: plan.HashBytes([]byte("old")), Owners: owners},
		{Path: "/home/refresh", Kind: state.Generated, WrittenHash: plan.HashBytes([]byte("same")), Owners: []state.Owner{{Primitive: "instructions", AgentInstance: "fake:main"}}},
	}
	return req
}

func writeFixture(t *testing.T, h *fake.Host, name, content string) {
	t.Helper()
	if err := h.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	filename := filepath.Join("testdata", name)
	if flag.Lookup("update").Value.String() == "true" {
		if err := os.WriteFile(filename, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}
