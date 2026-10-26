package embedded

import (
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestReadIdentityUsesGitConfigWithoutFallback(t *testing.T) {
	t.Parallel()
	w := setup(t)
	id, err := w.b.ReadIdentity(t.Context(), w.clone)
	if err != nil || id != (vcs.Identity{}) {
		t.Fatalf("KitBag fallback leaked into Git hint: %v %v", id, err)
	}
	put(t, filepath.Join(w.home, ".gitconfig"), "[user]\nname = Global Author\nemail = global@example.invalid\n")
	r, err := git.PlainOpen(w.clone)
	must(t, err)
	cfg, err := r.Config()
	must(t, err)
	cfg.User.Email = "local@example.invalid"
	must(t, r.SetConfig(cfg))
	id, err = w.b.ReadIdentity(t.Context(), w.clone)
	if err != nil || id != (vcs.Identity{Name: "Global Author", Email: "local@example.invalid"}) {
		t.Fatal(id, err)
	}
	put(t, filepath.Join(w.home, ".gitconfig"), "[include]\npath = /private\n")
	if _, err = w.b.ReadIdentity(t.Context(), w.clone); err == nil {
		t.Fatal("unsupported include returned a misleading identity")
	}
}
