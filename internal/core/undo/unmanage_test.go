package undo_test

import (
	"maps"
	"strconv"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

func TestUnmanageRestoresAfterHistoryRotation(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	var err error
	w.store, err = state.New(w.h, w.h, state.Options{HistoryLimit: 1})
	must(t, err)
	w.sync(t)
	for i := range 3 {
		w.decls[3] = generated("CODEX.md", "revision "+strconv.Itoa(i))
		w.sync(t)
	}
	db := w.database(t)
	if len(undo.History(db)) != 1 || len(db.Backups) != 3 {
		t.Fatalf("history rotation lost original metadata: %+v", db)
	}
	outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
	must(t, err)
	if outcome.Result.Code() != apply.OK {
		t.Fatalf("unmanage = %+v", outcome.Result)
	}
	content(t, w.h, "/home/a/tool", "mine")
	content(t, w.h, "/home/a/AGENTS.md", "own rules\n")
	content(t, w.h, "/home/a/settings", "local-key\n")
	absent(t, w.h, "/home/a/skill")
	absent(t, w.h, "/home/a/CODEX.md")
	w.sync(t)
	if result := w.sync(t); len(result.Changed) != 0 {
		t.Fatalf("not converged: %+v", result)
	}
}

func TestUnmanageRestoresOriginalsAndConverges(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	before := snapshot(t, w.h, "/home")
	dry, err := w.unmanage(t, w.h, undo.Options{DryRun: true})
	must(t, err)
	want := map[string]plan.Kind{"skill": plan.Discard, "tool": plan.Restore, "AGENTS.md": plan.Restore, "CODEX.md": plan.Discard, "settings": plan.Restore}
	if got := kinds(dry.Plan); !maps.Equal(got, want) {
		t.Fatalf("plan = %v; want %v", got, want)
	}
	if after := snapshot(t, w.h, "/home"); !maps.Equal(before, after) {
		t.Fatal("dry run changed the tree")
	}
	if _, err = w.unmanage(t, w.h, undo.Options{}); err == nil {
		t.Fatal("unmanage without a clock succeeded")
	}
	outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
	must(t, err)
	if outcome.Result.Code() != apply.OK || len(outcome.Result.Changed) != len(want) {
		t.Fatalf("unmanage = %+v", outcome.Result)
	}
	assertUnmanaged(t, w)
	db := w.database(t)
	groups := undo.History(db)
	if len(db.Targets) != 0 || len(groups) != 2 || len(groups[0].Paths) != len(want) {
		t.Fatalf("state after unmanage: %+v / %+v", db.Targets, groups)
	}
	// A fresh sync converges and backs up the restored originals again.
	w.sync(t)
	if result := w.sync(t); len(result.Changed) != 0 {
		t.Fatalf("not converged: %+v", result)
	}
	linked(t, w.h, "/home/a/skill", "/repo/skill")
	content(t, w.h, "/home/a/AGENTS.md", "generated\n")
}

func assertUnmanaged(t *testing.T, w *world) {
	t.Helper()
	absent(t, w.h, "/home/a/skill")
	absent(t, w.h, "/home/a/CODEX.md")
	content(t, w.h, "/home/a/tool", "mine")
	content(t, w.h, "/home/a/AGENTS.md", "own rules\n")
	content(t, w.h, "/home/a/settings", "local-key\n")
	noReserved(t, w.h, "/home/a")
}

func TestUnmanageUnmergesSettingsWithAbsorbedEdits(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	// A local edit is absorbed by the next merge, so the original is stale.
	write(t, w.h, "/home/a/settings", "local-key\nadded\nrepo-key"+managed+"\n")
	w.sync(t)
	outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
	must(t, err)
	if kinds(outcome.Plan)["settings"] != plan.Unmerge || outcome.Result.Code() != apply.OK {
		t.Fatalf("unmanage = %+v", outcome)
	}
	content(t, w.h, "/home/a/settings", "local-key\nadded\n")
	if len(w.database(t).Targets) != 0 {
		t.Fatal("ownership kept")
	}
}

func TestUnmanageProtectsChangedAndUnownedEntries(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.decls = append(w.decls, generated("../b/nested", "nested\n"))
	w.decls[len(w.decls)-1].Path = "/home/b/nested"
	w.sync(t)
	write(t, w.h, "/home/a/CODEX.md", "edited\n")
	must(t, w.h.Remove("/home/a/AGENTS.md"))
	must(t, w.h.Rename("/home/b", "/home/c"))
	must(t, w.h.Link("/home/c", "/home/b"))
	outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
	must(t, err)
	got := kinds(outcome.Plan)
	if got["CODEX.md"] != plan.Skip || got["AGENTS.md"] != plan.Release || got["nested"] != plan.Skip || outcome.Result.Code() != apply.Conflict {
		t.Fatalf("unmanage = %v, %+v", got, outcome.Result)
	}
	content(t, w.h, "/home/a/CODEX.md", "edited\n")
	content(t, w.h, "/home/c/nested", "nested\n")
	remaining := map[string]bool{}
	for _, owned := range w.database(t).Targets {
		remaining[owned.Path] = true
	}
	if !remaining["/home/a/CODEX.md"] || !remaining["/home/b/nested"] || remaining["/home/a/AGENTS.md"] || len(remaining) != 2 {
		t.Fatalf("ownership = %v", remaining)
	}
}

func TestUnmanageSettingsNeedTheirAdapter(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.decls = []target.Target{merged("repo-key\n")}
	w.sync(t)
	var outcome undo.Outcome
	must(t, w.store.WithLock(func(session *state.Session) error {
		var err error
		outcome, err = undo.Unmanage(w.h, session, nil, run(undo.Options{}))
		return err
	}))
	if kinds(outcome.Plan)["settings"] != plan.Skip || outcome.Result.Code() != apply.Conflict {
		t.Fatalf("unmanage = %+v", outcome)
	}
	content(t, w.h, "/home/a/settings", "local-key\nrepo-key"+managed+"\n")
}

func TestUnmanageUnparsableOrUnownedSettings(t *testing.T) {
	t.Parallel()
	for name, fragment := range map[string]string{"unparsable": "unparsable\n", "nothing owned": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			w.decls = []target.Target{merged(fragment)}
			must(t, w.h.Remove("/home/a/settings"))
			w.sync(t)
			outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
			must(t, err)
			wantKind := map[string]plan.Kind{"unparsable": plan.Skip, "nothing owned": plan.Release}[name]
			if got := kinds(outcome.Plan)["settings"]; got != wantKind {
				t.Fatalf("settings = %s; want %s", got, wantKind)
			}
		})
	}
}

func TestUnmanageInterruptionConverges(t *testing.T) {
	t.Parallel()
	const maxMutations = 60
	for n := 1; n < maxMutations; n++ {
		w := newWorld(t)
		w.sync(t)
		interrupted := &crashFS{FS: w.h, remaining: n}
		if _, err := w.unmanage(t, interrupted, run(undo.Options{})); err == nil {
			t.Logf("all %d mutation boundaries converged", n-1)
			return
		}
		outcome, err := w.unmanage(t, w.h, run(undo.Options{}))
		if err != nil || outcome.Result.Code() != apply.OK {
			t.Fatalf("after interruption %d: %+v, %v", n, outcome.Result, err)
		}
		assertUnmanaged(t, w)
		if db := w.database(t); len(db.Targets) != 0 || db.Pending != nil {
			t.Fatalf("after interruption %d: %+v", n, db)
		}
	}
	t.Fatal("did not reach the clean completion boundary")
}
