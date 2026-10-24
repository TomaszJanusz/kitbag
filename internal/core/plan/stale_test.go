package plan_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestMoveSkillDirectoryInOnePlan(t *testing.T) {
	t.Parallel()
	for _, mode := range []state.Kind{state.Link, state.Copy} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			old := declaration("old")
			write(t, h, filepath.Join(old.SourcePath, "SKILL.md"), "skill")
			req := request(old)
			applyTestPlan(t, h, &req, build(t, h, req), mode)
			moved := declaration("new")
			if err := h.Rename(old.SourcePath, moved.SourcePath); err != nil {
				t.Fatal(err)
			}
			req.Adapters = request(moved).Adapters
			got := build(t, h, req)
			assertKinds(t, got, plan.Remove, plan.Materialize)
			if got.Operations[0].Path != old.Path || got.Operations[1].Path != moved.Path {
				t.Fatal(got.Text())
			}
			applyTestPlan(t, h, &req, got, mode)
			assertKinds(t, build(t, h, req))
		})
	}
}

func TestStaleRemovalProtectsDrift(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unchanged", "missing", "edited", "foreign link", "directory edited"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("stale")
			write(t, h, d.SourcePath, "old")
			write(t, h, d.Path, "old")
			req := request()
			req.State.Targets = []state.Target{tracked(d, plan.HashBytes([]byte("old")), state.Copy)}
			prepareStale(t, h, d, scenario)
			got := build(t, h, req)
			want := plan.Remove
			if scenario == "edited" || scenario == "foreign link" || scenario == "directory edited" {
				want = plan.Skip
			}
			assertKinds(t, got, want)
			if want == plan.Skip {
				assertClass(t, got, plan.Conflict)
			}
		})
	}
}

func prepareStale(t *testing.T, h *fake.Host, d target.Target, scenario string) {
	t.Helper()
	switch scenario {
	case "missing":
		removeTestTree(t, h, d.Path)
	case "edited":
		write(t, h, d.Path, "local edit")
	case "foreign link":
		link(t, h, path("outside"), d.Path)
	case "directory edited":
		removeTestTree(t, h, d.Path)
		write(t, h, filepath.Join(d.Path, "unmanaged"), "local")
	}
}

func TestOverlappingMoveRemovesBeforeCreating(t *testing.T) {
	t.Parallel()
	h := fake.New()
	old := declaration("parent")
	write(t, h, old.SourcePath, "old")
	req := request(old)
	applyTestPlan(t, h, &req, build(t, h, req), state.Link)
	next := declaration("parent/child")
	next.SourcePath = path("repo", "new-source")
	write(t, h, next.SourcePath, "new")
	req.Adapters = request(next).Adapters
	got := build(t, h, req)
	assertKinds(t, got, plan.Remove, plan.Materialize)
	applyTestPlan(t, h, &req, got, state.Link)
	assertKinds(t, build(t, h, req))
}

func TestDriftedStaleAncestorBlocksCreate(t *testing.T) {
	t.Parallel()
	h := fake.New()
	old := declaration("parent")
	next := declaration("child")
	next.Path = filepath.Join(old.Path, "child")
	write(t, h, old.Path, "local")
	write(t, h, next.SourcePath, "new")
	req := request(next)
	req.State.Targets = []state.Target{tracked(old, plan.HashBytes([]byte("old")), state.Copy)}
	got := build(t, h, req)
	assertKinds(t, got, plan.Skip, plan.Skip)
	for _, op := range got.Operations {
		if op.Classification != plan.Conflict {
			t.Fatal(got.Text())
		}
	}
}

func TestCaseOnlyMoveRemovesBeforeCreating(t *testing.T) {
	t.Parallel()
	h := fake.New()
	old := declaration("Skill")
	write(t, h, old.SourcePath, "skill")
	req := request(old)
	applyTestPlan(t, h, &req, build(t, h, req), state.Copy)
	next := declaration("skill")
	next.SourcePath = old.SourcePath
	req.Adapters = request(next).Adapters
	got := build(t, h, req)
	assertKinds(t, got, plan.Remove, plan.Materialize)
	applyTestPlan(t, h, &req, got, state.Copy)
	assertKinds(t, build(t, h, req))
}

func TestDisappearedDeclaredSourceRemovesUnchangedTarget(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("gone")
	write(t, h, d.SourcePath, "old")
	req := request(d)
	applyTestPlan(t, h, &req, build(t, h, req), state.Copy)
	removeTestTree(t, h, d.SourcePath)
	got := build(t, h, req)
	assertKinds(t, got, plan.Remove)
	applyTestPlan(t, h, &req, got, state.Copy)
	// Missing sources stay absent from desired state after ownership is removed.
	assertKinds(t, build(t, h, req))
}
