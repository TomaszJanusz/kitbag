package vcs_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (s suite) testReadIdentity(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	reader, ok := w.backend.(vcs.IdentityReader)
	if !ok {
		t.Fatal("backend has no settings identity reader")
	}
	w.git(t, w.clone, "config", "user.name", "Local Author")
	w.git(t, w.clone, "config", "user.email", "local@example.invalid")
	id, err := reader.ReadIdentity(t.Context(), w.clone)
	mustNil(t, "read identity", err)
	if id != (vcs.Identity{Name: "Local Author", Email: "local@example.invalid"}) {
		t.Fatal(id)
	}
	if _, err = reader.ReadIdentity(t.Context(), filepath.Join(w.clone, "missing")); err == nil {
		t.Fatal("read identity outside a repository")
	}
}
