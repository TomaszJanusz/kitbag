package vcs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestRepositoryInitialization(t *testing.T) {
	t.Parallel()
	for name, factory := range map[string]func(*testing.T) subject{"system": newSystemSubject, "embedded": newEmbeddedSubject} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sub := factory(t)
			initializer, ok := sub.backend.(vcs.Initializer)
			if !ok {
				t.Fatal("missing initializer")
			}
			dir := t.TempDir()
			mustNil(t, "init", initializer.Init(t.Context(), dir))
			head, err := sub.backend.Head(t.Context(), dir)
			mustNil(t, "head", err)
			if head.Branch != "main" || !head.Unborn() {
				t.Fatalf("unexpected head: %+v", head)
			}
			if err = initializer.Init(t.Context(), dir); err == nil {
				t.Fatal("reinitialized existing repository")
			}
			metadata := filepath.Join(dir, ".git", "HEAD")
			before, err := os.ReadFile(metadata) //nolint:gosec // G304: metadata is the fixed .git/HEAD path inside t.TempDir, never user input.
			mustNil(t, "read HEAD", err)
			cancelled, cancel := context.WithCancel(t.Context())
			cancel()
			if err = initializer.Init(cancelled, dir); !errors.Is(err, cancelled.Err()) {
				t.Fatalf("ignored cancellation: %v", err)
			}
			after, err := os.ReadFile(metadata) //nolint:gosec // G304: metadata is the fixed .git/HEAD path inside t.TempDir, never user input.
			mustNil(t, "reread HEAD", err)
			if string(after) != string(before) {
				t.Fatal("changed existing HEAD")
			}
		})
	}
}
