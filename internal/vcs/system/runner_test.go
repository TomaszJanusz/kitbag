//go:build darwin || linux

package system_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

func TestExecRunnerLookPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // G306: the file must be executable.
		t.Fatal(err)
	}
	notExec := filepath.Join(dir, "plain")
	if err := os.WriteFile(notExec, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := system.ExecRunner{}
	pathList := strings.Join([]string{"", ".", "relative", t.TempDir(), dir}, string(filepath.ListSeparator))
	if got, err := r.LookPath("tool", pathList); err != nil || got != tool {
		t.Errorf("LookPath = %q, %v; want %q", got, err, tool)
	}
	for _, name := range []string{"plain", "missing"} {
		if got, err := r.LookPath(name, pathList); !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("LookPath(%q) = %q, %v; want not found", name, got, err)
		}
	}
	if got, err := r.LookPath("tool", "relative-only"); err == nil {
		t.Errorf("LookPath via a relative PATH entry = %q", got)
	}
}

func TestExecRunnerRun(t *testing.T) {
	t.Parallel()

	r := system.ExecRunner{}
	// A nil Env must mean an empty environment, never kitbag's own.
	res, err := r.Run(t.Context(), system.Command{Path: "/usr/bin/env", Dir: t.TempDir()})
	if err != nil || len(res.Stdout) != 0 || res.ExitCode != 0 {
		t.Errorf("env with nil Env = %q, %d, %v; want an empty environment", res.Stdout, res.ExitCode, err)
	}
	res, err = r.Run(t.Context(), system.Command{
		Path: "/bin/sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}, Dir: t.TempDir(),
	})
	if err != nil || string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" || res.ExitCode != 3 {
		t.Errorf("Run = %q %q %d, %v", res.Stdout, res.Stderr, res.ExitCode, err)
	}
	if _, err := r.Run(t.Context(), system.Command{Path: filepath.Join(t.TempDir(), "missing"), Dir: "/"}); err == nil {
		t.Error("running a missing executable succeeded")
	}
}

// TestNoInheritedGitEnvironment proves that GIT_* variables of kitbag's own
// process and of the platform environment never reach Git: with GIT_DIR and
// GIT_WORK_TREE pointing at another repository, the backend still reports
// the working copy it was asked about. It cannot run in parallel because it
// changes the process environment.
func TestNoInheritedGitEnvironment(t *testing.T) { //nolint:paralleltest // t.Setenv forbids t.Parallel.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	home, root := t.TempDir(), t.TempDir()
	repo, decoy := filepath.Join(root, "repo"), filepath.Join(root, "decoy")
	for _, dir := range []string{repo, decoy} {
		cmd := exec.CommandContext(t.Context(), gitPath, "init", "--quiet", dir) //nolint:gosec // G204: test fixture.
		cmd.Env = []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1"}
		if out, initErr := cmd.CombinedOutput(); initErr != nil {
			t.Fatalf("git init: %v\n%s", initErr, out)
		}
	}
	if err = os.WriteFile(filepath.Join(decoy, "decoy.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	gitEnv := map[string]string{"GIT_DIR": filepath.Join(decoy, ".git"), "GIT_WORK_TREE": decoy}
	for k, v := range gitEnv {
		t.Setenv(k, v)
	}
	gitEnv["HOME"], gitEnv["PATH"] = home, filepath.Dir(gitPath)

	b, err := system.New(t.Context(), system.Options{Env: fake.New(fake.WithEnv(gitEnv)), NoSystemConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Clean() {
		t.Errorf("status of %s = %+v; the decoy repository leaked in", repo, st.Changes)
	}
	if _, err := b.RemoteURL(t.Context(), repo, vcs.DefaultRemote); !errors.Is(err, vcs.ErrNoRemote) {
		t.Errorf("RemoteURL error = %v, want ErrNoRemote", err)
	}
}
