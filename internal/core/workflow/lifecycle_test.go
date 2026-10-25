package workflow_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
)

func TestAdoptNeedsLegacyRootsAndClock(t *testing.T) {
	t.Parallel()
	f := setup(t)
	if _, err := f.s.Adopt(false, false); !errors.Is(err, fs.ErrInvalid) || !strings.Contains(err.Error(), "legacy_roots") {
		t.Fatalf("adopt without roots = %v", err)
	}
	f.s.Config.Migration.LegacyRoots = []string{"~/repo"}
	f.s.Now = nil
	if _, err := f.s.Adopt(false, false); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("adopt without clock = %v", err)
	}
}

func TestLifecycleCommands(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.s.Config.Migration.LegacyRoots = []string{"~/repo"}
	home := filepath.Dir(f.s.Repository)
	output := filepath.Join(home, "output")
	if err := f.host.Link(filepath.Join(f.s.Repository, "source"), output); err != nil {
		t.Fatal(err)
	}
	assertAdopted(t, f, output)
	history, err := f.s.History()
	if err != nil || len(history) != 1 || !history[0].Adopted {
		t.Fatalf("history = %+v, %v", history, err)
	}
	reverted, err := f.s.Revert(1, false, false)
	if err != nil || reverted.Outcome.Result.Code() != apply.OK || reverted.Outcome.Plan.Operations[0].Kind != plan.Release {
		t.Fatalf("revert adoption = %+v, %v", reverted, err)
	}
	syncOK(t, f, true)
	assertUnmanage(t, f, output)
}

func assertAdopted(t *testing.T, f *fixture, output string) {
	t.Helper()
	adopted, err := f.s.Adopt(false, false)
	if err != nil || len(adopted.Report.Adopted) != 1 || adopted.Report.Adopted[0].Path != output {
		t.Fatalf("adopt = %+v, %v", adopted, err)
	}
}

func assertUnmanage(t *testing.T, f *fixture, output string) {
	t.Helper()
	dry, err := f.s.Unmanage(true)
	if err != nil || len(dry.Outcome.Plan.Operations) != 1 || dry.Outcome.Plan.Operations[0].Kind != plan.Discard {
		t.Fatalf("unmanage dry run = %+v, %v", dry, err)
	}
	done, err := f.s.Unmanage(false)
	if err != nil || done.Outcome.Result.Code() != apply.OK {
		t.Fatalf("unmanage = %+v, %v", done, err)
	}
	if _, err = f.host.Lstat(output); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("output kept: %v", err)
	}
	f.write(t, "kitbag.toml", "schema=")
	if _, err = f.s.Unmanage(true); err == nil {
		t.Fatal("unmanage with an unreadable manifest succeeded")
	}
	if _, err = f.s.Adopt(false, true); err == nil {
		t.Fatal("adopt with an unreadable manifest succeeded")
	}
}
