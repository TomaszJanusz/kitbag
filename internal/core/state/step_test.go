package state_test

import (
	"errors"
	"io/fs"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func pendingStep() *state.Step {
	after := target("/dest/file")
	return &state.Step{ID: "step-1", At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Path: after.Path, Stage: after.Path + ".kitbag-stage.step-1", Action: "output", After: &after}
}

func TestPendingStepRoundTripAndValidation(t *testing.T) {
	t.Parallel()
	cases := invalidSteps()
	maps.Copy(cases, invalidUndoSteps())
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _, _ := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				db.Pending = pendingStep()
				if mutate == nil {
					if err := session.Save(db); err != nil {
						return err
					}
					if got := load(t, session); !reflect.DeepEqual(got.Pending, db.Pending) {
						t.Fatalf("pending = %+v", got.Pending)
					}
					return nil
				}
				mutate(db.Pending)
				if err := session.Save(db); !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("invalid pending saved: %v", err)
				}
				return nil
			})
		})
	}
}

func invalidSteps() map[string]func(*state.Step) {
	return map[string]func(*state.Step){
		"valid":            nil,
		"action":           func(s *state.Step) { s.Action = "unknown" },
		"missing output":   func(s *state.Step) { s.After = nil },
		"missing stage":    func(s *state.Step) { s.Stage = "" },
		"stage alias":      func(s *state.Step) { s.Stage = "/dest/somebody-else" },
		"stage descendant": func(s *state.Step) { s.Stage += "/child" },
		"backup alias":     func(s *state.Step) { s.Backup = "/dest/original" },
		"before kind":      func(s *state.Step) { s.Before.Kind = "device" },
		"before hash":      func(s *state.Step) { s.Before = state.Entry{Kind: "file", Hash: "wrong"} },
		"missing backup":   func(s *state.Step) { s.Before = state.Entry{Kind: "file", Hash: hash("a")} },
		"output path":      func(s *state.Step) { s.After.Path = "/other" },
		"previous path":    func(s *state.Step) { previous := target("/other"); s.Previous = &previous },
		"remove output":    func(s *state.Step) { s.Action = "remove" },
		"empty ID":         func(s *state.Step) { s.ID = "" },
	}
}

// invalidUndoSteps covers discard, restore and adoption-move metadata.
func invalidUndoSteps() map[string]func(*state.Step) {
	return map[string]func(*state.Step){
		"output restored": func(s *state.Step) { s.Restored = &state.Entry{Kind: "link", Link: "/x"} },
		"remove release":  func(s *state.Step) { s.Action, s.After, s.Stage, s.Release = "remove", nil, "", true },
		"aside without previous entry": func(s *state.Step) {
			s.Discard = s.Path + ".kitbag-stage.old"
		},
		"backup and discard": func(s *state.Step) {
			s.Before = state.Entry{Kind: "file", Hash: hash("a")}
			s.Backup, s.Discard = s.Path+".before-kitbag.1", s.Path+".kitbag-stage.old"
		},
		"unowned discard": func(s *state.Step) {
			s.Before = state.Entry{Kind: "file", Hash: hash("a")}
			s.Discard = s.Path + ".kitbag-stage.old"
		},
		"discard alias": func(s *state.Step) {
			previous := target(s.Path)
			s.Before, s.Previous, s.Discard = state.Entry{Kind: "file", Hash: hash("a")}, &previous, "/dest/elsewhere"
		},
		"stage is discard": func(s *state.Step) {
			previous := target(s.Path)
			s.Before, s.Previous, s.Discard = state.Entry{Kind: "file", Hash: hash("a")}, &previous, s.Stage
		},
		"backup discard": func(s *state.Step) {
			previous := target(s.Path)
			s.Action, s.After, s.Stage = "backup", nil, ""
			s.Before, s.Previous, s.Discard = state.Entry{Kind: "file", Hash: hash("a")}, &previous, s.Path+".kitbag-stage.old"
		},
		"restore without entry": func(s *state.Step) { s.Action, s.After, s.Stage = "restore", nil, "" },
		"restore absent entry": func(s *state.Step) {
			s.Action, s.After, s.Stage, s.Restored = "restore", nil, "", &state.Entry{}
		},
		"restore file without source": func(s *state.Step) {
			s.Action, s.After, s.Stage, s.Restored = "restore", nil, "", &state.Entry{Kind: "file", Hash: hash("a")}
		},
		"restore link with source": func(s *state.Step) {
			s.Action, s.After, s.Stage, s.Restored, s.From = "restore", nil, "", &state.Entry{Kind: "link", Link: "/x"}, "/backup"
		},
		"restore relative source": func(s *state.Step) {
			s.Action, s.After, s.Stage, s.Restored, s.From = "restore", nil, "", &state.Entry{Kind: "file", Hash: hash("a")}, "backup"
		},
		"aside outside adoption": func(s *state.Step) {
			s.Action, s.After, s.Stage = "backup", nil, ""
			s.Before, s.Backup = state.Entry{Kind: "directory", Hash: hash("a")}, "/home/kit/.agents/elsewhere/skill"
		},
		"aside output": func(s *state.Step) {
			s.Before, s.Backup = state.Entry{Kind: "file", Hash: hash("a")}, "/home/kit/.agents/skills-before-kitbag/file"
		},
	}
}

func TestPendingStepVariants(t *testing.T) {
	t.Parallel()
	previous := target("/dest/file")
	for name, step := range map[string]*state.Step{
		"discard output": {Action: "output", Stage: "/dest/file.kitbag-stage.s", Discard: "/dest/file.kitbag-stage.s-old", Before: state.Entry{Kind: "file", Hash: hash("a")}, After: &previous, Previous: &previous, Release: true},
		"discard remove": {Action: "remove", Discard: "/dest/file.kitbag-stage.s", Before: state.Entry{Kind: "link", Link: "/repo/config"}, Previous: &previous},
		"restore backup": {Action: "restore", From: "/dest/file.before-kitbag.old", Restored: &state.Entry{Kind: "file", Hash: hash("c")}, Before: state.Entry{Kind: "file", Hash: hash("a")}, Discard: "/dest/file.kitbag-stage.s", Previous: &previous},
		"restore link":   {Action: "restore", Restored: &state.Entry{Kind: "link", Link: "../repo/old"}, After: &previous, Plan: "plan-1"},
		// A replaced link is journaled by its text: no backup, even when unowned.
		"discard unowned link": {Action: "output", Stage: "/dest/file.kitbag-stage.s", Discard: "/dest/file.kitbag-stage.s-old", Before: state.Entry{Kind: "link", Link: "/old/agent-config/file"}, After: &previous},
		"adoption aside":       {Action: "backup", Before: state.Entry{Kind: "directory", Hash: hash("a")}, Backup: "/home/kit/.agents/skills-before-kitbag/.claude/skills/file"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			step.ID, step.At, step.Path = "s", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), previous.Path
			store, _, _ := newStore(t, state.Options{})
			withLock(t, store, func(session *state.Session) error {
				db := snapshot()
				db.Pending = step
				if err := session.Save(db); err != nil {
					return err
				}
				if got := load(t, session); !reflect.DeepEqual(got.Pending, step) {
					t.Fatalf("pending = %+v", got.Pending)
				}
				return nil
			})
		})
	}
}
