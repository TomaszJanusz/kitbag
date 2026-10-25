package undo_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/target"
)

// badChange syncs a second plan: new instructions, a skill replaced by
// another, a settings fragment changed and the tool source edited.
func badChange(t *testing.T, w *world) {
	t.Helper()
	write(t, w.h, "/repo/other/SKILL.md", "other")
	write(t, w.h, "/repo/tool", "broken tool")
	w.decls = []target.Target{linkDecl("other"), fileDecl("tool"), generated("AGENTS.md", "broken\n"), generated("CODEX.md", "codex\n"), merged("bad-key\n")}
	w.sync(t)
}

func TestRevertRestoresPreviousTargets(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	good := w.database(t).Targets
	badChange(t, w)
	dry, err := w.revert(w.h, 1, undo.Options{DryRun: true})
	must(t, err)
	// The relinked tool (its source changed) only needs its old ownership.
	want := map[string]plan.Kind{"skill": plan.Restore, "other": plan.Discard, "AGENTS.md": plan.Restore, "settings": plan.Restore, "tool": plan.Record}
	if got := kinds(dry.Plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan = %v; want %v", got, want)
	}
	outcome, err := w.revert(w.h, 1, run(undo.Options{}))
	must(t, err)
	if outcome.Result.Code() != apply.OK {
		t.Fatalf("revert = %+v", outcome.Result)
	}
	linked(t, w.h, "/home/a/skill", "/repo/skill")
	absent(t, w.h, "/home/a/other")
	content(t, w.h, "/home/a/AGENTS.md", "generated\n")
	content(t, w.h, "/home/a/settings", "local-key\nrepo-key"+managed+"\n")
	if got := w.database(t).Targets; !reflect.DeepEqual(got, good) {
		t.Fatalf("ownership = %+v\nwant %+v", got, good)
	}
	// Reverting the revert recreates links; discarded output needs a sync.
	redo, err := w.revert(w.h, 1, run(undo.Options{}))
	must(t, err)
	if got := kinds(redo.Plan); got["other"] != plan.Restore || got["AGENTS.md"] != plan.Skip || redo.Result.Code() != apply.Warnings {
		t.Fatalf("redo = %v, %+v", got, redo.Result)
	}
	linked(t, w.h, "/home/a/other", "/repo/other")
}

func TestRevertProtectsChangedDestinations(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	badChange(t, w)
	write(t, w.h, "/home/a/AGENTS.md", "edited\n")
	outcome, err := w.revert(w.h, 1, run(undo.Options{}))
	must(t, err)
	if outcome.Result.Code() != apply.Conflict {
		t.Fatalf("revert = %+v", outcome.Result)
	}
	content(t, w.h, "/home/a/AGENTS.md", "edited\n")
	linked(t, w.h, "/home/a/skill", "/repo/skill")
	// The partial revert, the bad change and the first sync: back to the start.
	forced, err := w.revert(w.h, 3, run(undo.Options{Force: true}))
	must(t, err)
	if forced.Result.Code() != apply.OK || !forced.Plan.Force {
		t.Fatalf("forced = %+v", forced)
	}
	content(t, w.h, "/home/a/AGENTS.md", "own rules\n")
	content(t, w.h, "/home/a/tool", "mine")
	absent(t, w.h, "/home/a/skill")
	if len(w.database(t).Targets) != 0 {
		t.Fatal("ownership kept after reverting every plan")
	}
	kept := false
	for _, name := range names(t, w.h, "/home/a") {
		if data, readErr := w.h.ReadFile("/home/a/" + name); readErr == nil && string(data) == "edited\n" {
			kept = name != "AGENTS.md"
		}
	}
	if !kept {
		t.Fatalf("edited content not kept in a backup: %v", names(t, w.h, "/home/a"))
	}
}

func TestRevertRangeAndMissingBackup(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	if _, err := w.revert(w.h, 1, run(undo.Options{})); !undo.OutOfRange(err) {
		t.Fatalf("empty journal: %v", err)
	}
	w.sync(t)
	badChange(t, w)
	for _, to := range []int{0, 3} {
		if _, err := w.revert(w.h, to, run(undo.Options{})); !undo.OutOfRange(err) {
			t.Fatalf("--to %d: %v", to, err)
		}
	}
	removeLastBackups(t, w)
	outcome, err := w.revert(w.h, 1, run(undo.Options{DryRun: true}))
	must(t, err)
	skipped := 0
	for _, op := range outcome.Plan.Operations {
		if op.Kind == plan.Skip {
			skipped++
		}
	}
	if skipped == 0 {
		t.Fatalf("missing backup not skipped: %+v", outcome.Plan)
	}
}

func removeLastBackups(t *testing.T, w *world) {
	t.Helper()
	db := w.database(t)
	last := db.History[len(db.History)-1].PlanID()
	for _, entry := range db.History {
		for _, change := range entry.Changes {
			if entry.PlanID() == last && change.Previous != nil && change.Previous.BackupPath != "" {
				must(t, w.h.Remove(change.Previous.BackupPath))
			}
		}
	}
}

func TestHistoryGroupsPlans(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	badChange(t, w)
	var listed []undo.Applied
	must(t, w.store.WithLock(func(session *state.Session) error {
		var err error
		listed, err = undo.List(session)
		return err
	}))
	if len(listed) != 2 || listed[0].Number != 1 || listed[1].Number != 2 || listed[0].Adopted || len(listed[1].Paths) != len(w.decls) {
		t.Fatalf("history = %+v", listed)
	}
}

func TestRevertRequiresRecoveredState(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.sync(t)
	db := w.database(t)
	db.Pending = &state.Step{ID: "x", At: clock(), Path: "/home/a/new", Action: "remove", Plan: "x"}
	must(t, w.store.WithLock(func(session *state.Session) error { return session.Save(db) }))
	if _, err := w.revert(w.h, 1, undo.Options{DryRun: true}); err == nil {
		t.Fatal("dry run planned over a pending step")
	}
	if _, err := w.revert(w.h, 1, run(undo.Options{})); err == nil || !strings.Contains(err.Error(), "recovered") {
		t.Fatalf("recovery before revert: %v", err)
	}
	if db = w.database(t); db.Pending != nil {
		t.Fatal("pending step not recovered")
	}
}

func TestRevertInterruptionConverges(t *testing.T) {
	t.Parallel()
	const maxMutations = 60
	for n := 1; n < maxMutations; n++ {
		w := newWorld(t)
		w.sync(t)
		good := w.database(t).Targets
		badChange(t, w)
		interrupted := &crashFS{FS: w.h, remaining: n}
		if _, err := w.revert(interrupted, 1, run(undo.Options{})); err == nil {
			t.Logf("all %d mutation boundaries converged", n-1)
			return
		}
		// Recovery finishes or rolls back the interrupted step. Completed steps
		// form their own journaled plan, so reverting it together with the bad
		// change finishes the job; already restored entries need ownership only.
		if _, err := w.revert(w.h, 1, run(undo.Options{})); err != nil && !strings.Contains(err.Error(), "recovered an interrupted step") {
			t.Fatalf("after interruption %d: %v", n, err)
		}
		to := 1
		if len(undo.History(w.database(t))) == 3 {
			to = 2
		}
		outcome, err := w.revert(w.h, to, run(undo.Options{}))
		if err != nil || outcome.Result.Code() != apply.OK {
			t.Fatalf("after interruption %d: %+v, %v", n, outcome.Result, err)
		}
		linked(t, w.h, "/home/a/skill", "/repo/skill")
		absent(t, w.h, "/home/a/other")
		content(t, w.h, "/home/a/AGENTS.md", "generated\n")
		if got := w.database(t); !reflect.DeepEqual(got.Targets, good) || got.Pending != nil {
			t.Fatalf("after interruption %d: %+v", n, got)
		}
	}
	t.Fatal("did not reach the clean completion boundary")
}
