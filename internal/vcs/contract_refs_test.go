package vcs_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (s suite) testBranches(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	head := w.head(t, w.clone)
	ctx := t.Context()

	mustNil(t, "create", w.backend.CreateBranch(ctx, w.clone, "topic", ""))
	if err := w.backend.CreateBranch(ctx, w.clone, "topic", ""); err == nil {
		t.Error("creating an existing branch succeeded")
	}
	if h := w.head(t, w.clone); h != head {
		t.Errorf("create switched HEAD to %+v", h)
	}
	mustNil(t, "push branch", w.backend.Push(ctx, w.clone, vcs.PushOptions{Branch: "topic"}))
	if got := w.head(t, w.cloneBranch(t, "topic")); got.Branch != "topic" || got.Commit != head.Commit {
		t.Errorf("remote topic = %+v, want %s", got, head.Commit)
	}

	mustNil(t, "delete", w.backend.DeleteBranch(ctx, w.clone, "topic"))
	if err := w.backend.DeleteBranch(ctx, w.clone, "topic"); err == nil {
		t.Error("deleting a missing branch succeeded")
	}
	if err := w.backend.DeleteBranch(ctx, w.clone, "main"); err == nil {
		t.Error("deleting the current branch succeeded")
	}
	mustNil(t, "create at start", w.backend.CreateBranch(ctx, w.clone, "from-start", head.Commit))
}

// cloneBranch clones branch of the remote into a new directory.
func (w world) cloneBranch(t *testing.T, branch string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(w.remote), "branch-"+branch)
	mustNil(t, "clone "+branch, w.backend.Clone(t.Context(), w.remote, dir, vcs.CloneOptions{Branch: branch}))
	return dir
}

func (s suite) testRemoteURL(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	got, err := w.backend.RemoteURL(t.Context(), w.clone, vcs.DefaultRemote)
	mustNil(t, "remote url", err)
	if got != w.remote {
		t.Errorf("remote URL = %q, want %q", got, w.remote)
	}
	_, err = w.backend.RemoteURL(t.Context(), w.clone, "missing")
	mustIs(t, "missing remote", err, vcs.ErrNoRemote)
}

func (s suite) testDetachedHead(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	commit := w.head(t, w.clone).Commit
	w.git(t, w.clone, "checkout", "--quiet", "--detach", "HEAD")

	h := w.head(t, w.clone)
	if !h.Detached() || h.Commit != commit {
		t.Errorf("head = %+v, want detached at %s", h, commit)
	}
	_, err := vcs.Pull(t.Context(), w.backend, w.clone)
	mustIs(t, "pull", err, vcs.ErrDetachedHead)
	mustIs(t, "fast-forward", w.backend.FastForward(t.Context(), w.clone), vcs.ErrDetachedHead)
	mustIs(t, "push", w.backend.Push(t.Context(), w.clone, vcs.PushOptions{}), vcs.ErrDetachedHead)
}

func (s suite) testNoUpstream(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	mustNil(t, "create", w.backend.CreateBranch(t.Context(), w.clone, "local-only", ""))
	w.git(t, w.clone, "checkout", "--quiet", "local-only")

	_, err := vcs.Pull(t.Context(), w.backend, w.clone)
	mustIs(t, "pull", err, vcs.ErrNoUpstream)
	_, err = w.backend.RemoteHead(t.Context(), w.clone)
	mustIs(t, "probe without upstream", err, vcs.ErrNoUpstream)
	mustIs(t, "fast-forward", w.backend.FastForward(t.Context(), w.clone), vcs.ErrNoUpstream)
}

func (s suite) testUnreachableRemote(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	missing := filepath.Join(t.TempDir(), "missing.git")
	if err := w.backend.Clone(t.Context(), missing, filepath.Join(t.TempDir(), "c"), vcs.CloneOptions{}); err == nil {
		t.Error("cloning a missing remote succeeded")
	}
	w.git(t, w.clone, "remote", "set-url", vcs.DefaultRemote, missing)
	if _, err := w.backend.RemoteHead(t.Context(), w.clone); err == nil {
		t.Error("probing missing remote succeeded")
	}
	if err := w.backend.Fetch(t.Context(), w.clone); err == nil {
		t.Error("fetching from a missing remote succeeded")
	}
	if _, err := vcs.Pull(t.Context(), w.backend, w.clone); err == nil {
		t.Error("pulling from a missing remote succeeded")
	}
}

func (s suite) testPermissionDeniedRemote(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permission bits enforced for a non-root user")
	}
	w := s.newWorld(t)
	mustNil(t, "chmod", os.Chmod(w.remote, 0))
	t.Cleanup(func() {
		if err := os.Chmod(w.remote, 0o700); err != nil { //nolint:gosec // G302: a directory needs the execute bit to be removed afterwards.
			t.Errorf("restore permissions: %v", err)
		}
	})
	if err := w.backend.Fetch(t.Context(), w.clone); err == nil {
		t.Error("fetching from an unreadable remote succeeded")
	}
}

func (s suite) testNoRepositoryDiscovery(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	sub := filepath.Join(w.clone, "plain")
	mustNil(t, "mkdir", os.Mkdir(sub, 0o700))
	if st, err := w.backend.Status(t.Context(), sub); err == nil {
		t.Errorf("status of a directory inside the clone used the clone: %+v", st)
	}
	if _, err := w.backend.Head(t.Context(), t.TempDir()); err == nil {
		t.Error("head of a plain directory succeeded")
	}
}

func (s suite) testRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	ctx := t.Context()
	for name, err := range map[string]error{
		"relative dir":       w.backend.Fetch(ctx, "clone"),
		"unclean dir":        w.backend.Fetch(ctx, w.clone+"/../clone"),
		"option as revision": diffErr(w, "--output=/tmp/x", "HEAD"),
		"empty revision":     diffErr(w, "", "HEAD"),
		"option as branch":   w.backend.CreateBranch(ctx, w.clone, "-D", ""),
		"option as start":    w.backend.CreateBranch(ctx, w.clone, "ok", "--orphan"),
		"option as remote":   remoteErr(w, "--add"),
		"empty message":      commitErr(w, " \n"),
		"empty URL":          w.backend.Clone(ctx, "", filepath.Join(t.TempDir(), "x"), vcs.CloneOptions{}),
	} {
		if !isInvalid(err) {
			t.Errorf("%s: error = %v, want ErrInvalidArgument", name, err)
		}
	}
}
