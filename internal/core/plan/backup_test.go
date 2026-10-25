package plan_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestUnmanagedBackupIsOnceAndNeverOverwritten(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("settings")
	d.Backup = target.Once
	write(t, h, d.SourcePath, "desired")
	write(t, h, d.Path, "original")
	req := request(d)
	first := build(t, h, req)
	assertKinds(t, first, plan.Backup, plan.Materialize)
	applyTestPlan(t, h, &req, first, state.Copy)
	assertKinds(t, build(t, h, req))
	write(t, h, d.SourcePath, "updated")
	update := build(t, h, req)
	assertKinds(t, update, plan.Materialize)
	applyTestPlan(t, h, &req, update, state.Copy)
	assertKinds(t, build(t, h, req))
	backup, err := h.ReadFile(d.Path + ".before-kitbag.20260101T000000.000000000Z-0")
	if err != nil || string(backup) != "original" {
		t.Fatalf("backup = %s, %v", backup, err)
	}
	// Loss of ownership never authorizes replacing the retained original backup.
	req.State.Targets = nil
	assertKinds(t, build(t, h, req), plan.Skip)
}

func TestUnmanagedObstacles(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"refuse", "directory", "foreign link", "existing backup", "reserved backup", "identical unmanaged"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := fake.New()
			d := declaration("obstacle")
			d.Backup = target.Once
			write(t, h, d.SourcePath, "desired")
			req := request(d)
			prepareObstacle(t, h, &req, d, scenario)
			got := build(t, h, req)
			if scenario == "reserved backup" {
				assertKinds(t, got, plan.Materialize, plan.Skip)
			} else {
				assertKinds(t, got, plan.Skip)
			}
			if scenario != "reserved backup" {
				assertClass(t, got, plan.UnmanagedObstacle)
			}
		})
	}
}

func prepareObstacle(t *testing.T, h *fake.Host, req *plan.Request, d target.Target, scenario string) {
	t.Helper()
	write(t, h, d.Path, "desired")
	switch scenario {
	case "refuse", "identical unmanaged":
		d.Backup = target.Refuse
		req.Adapters = request(d).Adapters
	case "directory":
		removeTestTree(t, h, d.Path)
		write(t, h, d.Path+"/local", "local")
	case "foreign link":
		link(t, h, path("outside"), d.Path)
	case "existing backup":
		write(t, h, d.Path+".kitbag-backup", "original")
	case "reserved backup":
		other := declaration("other")
		other.Path = d.Path + ".kitbag-backup"
		write(t, h, other.SourcePath, "other")
		req.Adapters = request(d, other).Adapters
	}
}

func TestBackupResumeAfterRename(t *testing.T) {
	t.Parallel()
	h := fake.New()
	d := declaration("resume")
	d.Backup = target.Once
	write(t, h, d.SourcePath, "desired")
	write(t, h, d.Path, "original")
	if err := h.Rename(d.Path, d.Path+".kitbag-backup"); err != nil {
		t.Fatal(err)
	}
	req := request(d)
	got := build(t, h, req)
	assertKinds(t, got, plan.Materialize)
	applyTestPlan(t, h, &req, got, state.Copy)
	assertKinds(t, build(t, h, req))
}
