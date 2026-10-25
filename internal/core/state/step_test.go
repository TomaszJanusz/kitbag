package state_test

import (
	"errors"
	"io/fs"
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
	for name, mutate := range invalidSteps() {
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
