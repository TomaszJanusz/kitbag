package vcs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

// testGitConfig is the only Git configuration tests see: the system file is
// disabled and HOME is a temporary directory holding this file.
const testGitConfig = `[user]
	name = KitBag Test
	email = test@kitbag.invalid
[init]
	defaultBranch = main
[commit]
	gpgSign = false
[core]
	autocrlf = false
[advice]
	detachedHead = false
`

// TestSystemContract runs the backend contract against the git on the test
// process's PATH, with a temporary HOME and no system configuration.
func TestSystemContract(t *testing.T) {
	t.Parallel()
	runContract(t, newSystemSubject)
}

func newSystemSubject(t *testing.T) subject {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	mustNil(t, "find git", err)
	home := t.TempDir()
	mustNil(t, "write .gitconfig", os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(testGitConfig), 0o600))
	env := map[string]string{"HOME": home, "PATH": filepath.Dir(gitPath)}

	b, err := system.New(t.Context(), system.Options{Env: fake.New(fake.WithEnv(env)), NoSystemConfig: true})
	mustNil(t, "system backend", err)
	return subject{
		backend: b,
		git: func(t *testing.T, dir string, args ...string) {
			t.Helper()
			cmd := exec.CommandContext(t.Context(), gitPath, args...) //nolint:gosec // G204: test fixture runs the resolved Git with fixed arguments.
			cmd.Dir = dir
			cmd.Env = []string{"HOME=" + home, "PATH=" + env["PATH"], "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C"}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %q: %v\n%s", args, err, out)
			}
		},
	}
}

// TestSystemCommitRefusedUnstages proves that a commit refused by a hook
// leaves nothing staged, so the working copy shows only the edit itself.
func TestSystemCommitRefusedUnstages(t *testing.T) {
	t.Parallel()
	w := suite{newSubject: newSystemSubject}.newWorld(t)
	hooks := filepath.Join(w.clone, ".git", "hooks")
	mustNil(t, "hooks", os.MkdirAll(hooks, 0o700))
	mustNil(t, "hook", os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o700)) //nolint:gosec // G306: a Git hook must be executable.
	writeFile(t, w.clone, "seed.txt", "changed\n")
	if _, err := w.backend.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: "Refused"}); err == nil {
		t.Fatal("commit succeeded despite the refusing hook")
	}
	w.git(t, w.clone, "diff", "--cached", "--quiet")
	if st := w.status(t, w.clone); len(st.Changes) != 1 || st.Changes[0].Path != "seed.txt" {
		t.Fatalf("status after refused commit: %+v", st)
	}
}
