package vcs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded"
)

func TestBackendPreviewsIdentical(t *testing.T) {
	t.Parallel()
	w := (suite{newSubject: newSystemSubject}).newWorld(t)
	before := w.head(t, w.clone).Commit
	writeFile(t, w.clone, "seed.txt", "new\n")
	writeFile(t, w.clone, spacePath, "hello\nworld\n")
	after := w.commit(t, w.clone, "Preview")
	systemPreview, err := w.backend.Preview(t.Context(), w.clone, before, after)
	mustNil(t, "system preview", err)
	embeddedPreview, err := embedded.New(embedded.Options{}).Preview(t.Context(), w.clone, before, after)
	mustNil(t, "embedded preview", err)
	if systemPreview != embeddedPreview || !strings.Contains(systemPreview, "-seed\n+new") || !strings.Contains(systemPreview, "+hello\n+world") {
		t.Fatalf("system %q\nembedded %q", systemPreview, embeddedPreview)
	}
}

func (s suite) testShallowClone(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	writeFile(t, w.clone, "second.txt", "second\n")
	w.commit(t, w.clone, "Second")
	mustNil(t, "push", w.backend.Push(t.Context(), w.clone, vcs.PushOptions{}))
	dir := filepath.Join(t.TempDir(), "shallow")
	mustNil(t, "shallow clone", w.backend.Clone(t.Context(), w.remote, dir, vcs.CloneOptions{Depth: 1}))
	if st := w.status(t, dir); !st.Clean() || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("shallow status %+v", st)
	}
	// A shallow boundary is real Git metadata, not just an option ignored by Clone.
	boundary, readErr := os.ReadFile(filepath.Join(dir, ".git", "shallow")) //nolint:gosec // G304: the test-owned temporary clone, not user input.
	mustNil(t, "shallow boundary", readErr)
	if len(strings.TrimSpace(string(boundary))) != 40 {
		t.Fatalf("shallow boundary = %q", boundary)
	}
	writeFile(t, w.clone, "third.txt", "third\n")
	next := w.commit(t, w.clone, "Third")
	mustNil(t, "push third", w.backend.Push(t.Context(), w.clone, vcs.PushOptions{}))
	result, err := vcs.Pull(t.Context(), w.backend, dir)
	mustNil(t, "shallow pull", err)
	if result.Status.Commit != next || result.Outcome != vcs.PullUpdated {
		t.Fatalf("shallow pull %+v", result)
	}
}
