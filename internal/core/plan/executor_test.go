package plan_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

// applyTestPlan invokes production execution through a real locked state session.
func applyTestPlan(t *testing.T, h *fake.Host, req *plan.Request, p plan.Plan, mode state.Kind) {
	t.Helper()
	env := fake.New(fake.WithEnv(map[string]string{"HOME": path("apply-state")}))
	store, err := state.New(h, env, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithLock(func(session *state.Session) error {
		if saveErr := session.Save(req.State); saveErr != nil {
			return saveErr
		}
		_, applyErr := apply.Execute(h, session, p, apply.Options{Mode: string(mode), Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }})
		if applyErr != nil {
			return applyErr
		}
		req.State, applyErr = session.Load()
		return applyErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func removeTestTree(t *testing.T, h *fake.Host, name string) {
	t.Helper()
	info, err := h.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		entries, readErr := h.ReadDir(name)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			removeTestTree(t, h, filepath.Join(name, entry.Name()))
		}
	}
	if err = h.Remove(name); err != nil {
		t.Fatal(err)
	}
}
