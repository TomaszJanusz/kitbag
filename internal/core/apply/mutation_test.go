package apply_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// crashFS freezes writes after returning an error from a successful mutation,
// including post-rename durability errors. It shares exactly the fake's tree.
type crashFS struct {
	platform.FS
	remaining int
}

func (h *crashFS) mutation(fn func() error) error {
	if h.remaining == 0 {
		return errors.New("injected process interruption")
	}
	if err := fn(); err != nil {
		return err
	}
	h.remaining--
	if h.remaining == 0 {
		return errors.New("injected post-mutation interruption")
	}
	return nil
}

func (h *crashFS) MkdirAll(path string, mode fs.FileMode) error {
	return h.mutation(func() error { return h.FS.MkdirAll(path, mode) })
}

func (h *crashFS) WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	return h.mutation(func() error { return h.FS.WriteAtomic(path, data, mode) })
}

func (h *crashFS) Link(target, path string) error {
	return h.mutation(func() error { return h.FS.Link(target, path) })
}

func (h *crashFS) Rename(old, path string) error {
	return h.mutation(func() error { return h.FS.Rename(old, path) })
}

func (h *crashFS) Remove(path string) error {
	return h.mutation(func() error { return h.FS.Remove(path) })
}

func TestEveryFilesystemMutationInterruptionConverges(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"link", "copy"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			clean, cleanStore, cleanReq := recoveryWorld(t, mode)
			assertCode(t, run(t, clean, cleanStore, cleanReq, options(mode)), apply.OK)
			expected := database(t, cleanStore)
			testMutationPoints(t, mode, expected)
		})
	}
}

func testMutationPoints(t *testing.T, mode string, expected state.Database) {
	t.Helper()
	const maxMutations = 80
	for n := 1; n < maxMutations; n++ {
		h, _, req := recoveryWorld(t, mode)
		interrupted := &crashFS{FS: h, remaining: n}
		store, err := state.New(interrupted, h, state.Options{HistoryLimit: 100})
		must(t, err)
		_, err = apply.Run(interrupted, store, req, options(mode))
		if err == nil {
			t.Logf("%s: all %d filesystem mutation boundaries converged", mode, n-1)
			return
		}
		interrupted.remaining = -1
		assertCode(t, run(t, interrupted, store, req, options(mode)), apply.OK)
		compareRecovered(t, h, store, expected)
	}
	t.Fatal("did not reach clean completion boundary")
}
