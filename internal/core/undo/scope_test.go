package undo_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
)

// TestUnmanageOnlyLeavesOtherOutputs scopes unmanage to one path, as agent
// edits do for a directory the manifest stopped declaring.
func TestUnmanageOnlyLeavesOtherOutputs(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	only := func(path string) bool { return path == "/home/a/AGENTS.md" }
	outcome, err := w.unmanage(t, w.h, run(undo.Options{Only: only}))
	must(t, err)
	if got := kinds(outcome.Plan); !maps.Equal(got, map[string]plan.Kind{"AGENTS.md": plan.Restore}) {
		t.Fatalf("plan = %v", got)
	}
	content(t, w.h, "/home/a/AGENTS.md", "own rules\n")
	content(t, w.h, "/home/a/CODEX.md", "codex\n")
	linked(t, w.h, "/home/a/skill", "/repo/skill")
	if db := w.database(t); len(db.Targets) != len(w.decls)-1 {
		t.Fatalf("ownership outside the scope changed: %+v", db.Targets)
	}
}

// TestUnmanageReleaseChanged forgets a changed output instead of skipping it,
// and never touches its content.
func TestUnmanageReleaseChanged(t *testing.T) {
	t.Parallel()
	for _, release := range []bool{false, true} {
		w := newWorld(t)
		w.sync(t)
		write(t, w.h, "/home/a/CODEX.md", "edited by hand\n")
		only := func(path string) bool { return path == "/home/a/CODEX.md" }
		outcome, err := w.unmanage(t, w.h, run(undo.Options{Only: only, ReleaseChanged: release}))
		must(t, err)
		op := outcome.Plan.Operations[0]
		want, warning := plan.Skip, "left alone"
		if release {
			want, warning = plan.Release, "no longer managed"
		}
		if op.Kind != want || !strings.Contains(op.Warning, warning) || owns(t, w, "/home/a/CODEX.md") == release {
			t.Fatalf("release %t: %+v", release, op)
		}
		content(t, w.h, "/home/a/CODEX.md", "edited by hand\n")
	}
}

func owns(t *testing.T, w *world, path string) bool {
	t.Helper()
	for _, target := range w.database(t).Targets {
		if target.Path == path {
			return true
		}
	}
	return false
}
