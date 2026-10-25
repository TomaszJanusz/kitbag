package instructions_test

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type declared []target.Target

func (d declared) Declare(manifest.FileSystem, string, *manifest.Manifest) ([]target.Target, error) {
	return d, nil
}

func build(t *testing.T, h *fake.Host, targets ...target.Target) plan.Plan {
	t.Helper()
	p, err := plan.Build(h, plan.Request{
		Manifest:   &manifest.Manifest{Schema: 1},
		Repository: repository,
		Adapters:   []target.Adapter{declared(targets)},
		State:      state.Database{SchemaVersion: state.CurrentVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func kinds(p plan.Plan) string {
	names := make([]string, 0, len(p.Operations))
	for _, op := range p.Operations {
		names = append(names, string(op.Kind))
	}
	return strings.Join(names, ",")
}

// TestPlannerAcceptsDeclarations proves every strategy yields a declaration the
// planner accepts and that an unmanaged destination is backed up once by the
// planner (Backup: Once), not by this package.
func TestPlannerAcceptsDeclarations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"import-host", "concat-host", "link"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := fixtureHost(t)
			d := declare(t, name).Target
			if got := kinds(build(t, h, d)); got != "materialize" && got != "write" {
				t.Fatalf("missing destination: %s", got)
			}
			writeFile(t, h, d.Path, []byte("# Hand-written instructions\n"))
			p := build(t, h, d)
			if got := kinds(p); got != "backup,materialize" && got != "backup,write" {
				t.Fatalf("unmanaged destination: %s", got)
			}
			if p.Operations[0].BackupPath != d.Path+".before-kitbag." {
				t.Fatalf("backup path %q", p.Operations[0].BackupPath)
			}
		})
	}
}

func TestUnsupportedInputsAreSkipped(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"link-host", "missing-source"} {
		h := fixtureHost(t)
		p := build(t, h, declare(t, name).Target)
		if kinds(p) != "skip" || p.Operations[0].Warning == "" {
			t.Fatalf("%s: %s", name, kinds(p))
		}
	}
	h := fixtureHost(t)
	if err := h.Link(repository+"/global/AGENTS.md", repository+"/global/link.md"); err != nil {
		t.Fatal(err)
	}
	if err := h.MkdirAll(repository+"/hosts/dir.md", 0o700); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*manifest.Instructions){
		func(in *manifest.Instructions) { in.Source = "global/link.md" },
		func(in *manifest.Instructions) { in.Hosts = "hosts/dir.md" },
	} {
		req := goldenCases()[3].request()
		mutate(req.Instructions)
		got, err := instructions.Declare(h, req)
		if err != nil || !strings.Contains(got.Target.Warning, "not a regular file") {
			t.Fatalf("non-regular input: %+v, %v", got, err)
		}
	}
}
