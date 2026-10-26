package vcs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// subject is one backend prepared for a single test, with an isolated Git
// configuration (fixed identity, branch main, no signing).
type subject struct {
	backend vcs.Backend
	// git runs a Git command in dir for arranging situations the interface
	// cannot create, such as a bare repository or a detached HEAD.
	git func(t *testing.T, dir string, args ...string)
}

// factory builds a fresh subject.
type factory func(t *testing.T) subject

// suite runs the backend contract against one implementation. Every case is
// a method so that failures point at the case, and every case runs in
// parallel on its own repositories below t.TempDir.
type suite struct {
	newSubject factory
}

// runContract is the contract every backend (system now, embedded later)
// must satisfy.
func runContract(t *testing.T, newSubject factory) {
	t.Helper()
	s := suite{newSubject: newSubject}
	t.Run("CloneEmptyIsUnborn", s.testCloneEmptyIsUnborn)
	t.Run("ShallowClone", s.testShallowClone)
	t.Run("CommitPushClone", s.testCommitPushClone)
	t.Run("NothingToCommit", s.testNothingToCommit)
	t.Run("StatusChanges", s.testStatusChanges)
	t.Run("DiffOfTrees", s.testDiffOfTrees)
	t.Run("PullFastForwards", s.testPullFastForwards)
	t.Run("PullIntoUnborn", s.testPullIntoUnborn)
	t.Run("PullSkipsDirty", s.testPullSkipsDirty)
	t.Run("PullLeavesDiverged", s.testPullLeavesDiverged)
	t.Run("PushRejectsNonFF", s.testPushRejectsNonFastForward)
	t.Run("DropCommit", s.testDropCommit)
	t.Run("Branches", s.testBranches)
	t.Run("ReadIdentity", s.testReadIdentity)
	t.Run("RemoteURL", s.testRemoteURL)
	t.Run("RemoteHead", s.testRemoteHead)
	t.Run("DetachedHead", s.testDetachedHead)
	t.Run("NoUpstream", s.testNoUpstream)
	t.Run("UnreachableRemote", s.testUnreachableRemote)
	t.Run("PermissionDeniedRemote", s.testPermissionDeniedRemote)
	t.Run("NoRepositoryDiscovery", s.testNoRepositoryDiscovery)
	t.Run("RejectsInvalidArguments", s.testRejectsInvalidArguments)
}

// world is a bare remote with one commit on main and a clone of it.
type world struct {
	subject
	remote, clone string
}

// newWorld creates a bare remote at <tmp>/remote.git seeded with one commit
// holding seed.txt, and a clone of it at <tmp>/clone.
func (s suite) newWorld(t *testing.T) world {
	t.Helper()
	sub := s.newSubject(t)
	root := t.TempDir()
	w := world{subject: sub, remote: filepath.Join(root, "remote.git"), clone: filepath.Join(root, "clone")}
	sub.git(t, root, "init", "--quiet", "--bare", "--initial-branch=main", w.remote)

	seed := w.cloneAt(t, "seed")
	writeFile(t, seed, "seed.txt", "seed\n")
	w.commit(t, seed, "Seed")
	mustNil(t, "push seed", sub.backend.Push(t.Context(), seed, vcs.PushOptions{SetUpstream: true}))

	mustNil(t, "clone", sub.backend.Clone(t.Context(), w.remote, w.clone, vcs.CloneOptions{}))
	return w
}

// cloneAt clones the remote into a new sibling directory called name.
func (w world) cloneAt(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(w.remote), name)
	mustNil(t, "clone "+name, w.backend.Clone(t.Context(), w.remote, dir, vcs.CloneOptions{}))
	return dir
}

// commit records every change in dir and returns the new commit.
func (w world) commit(t *testing.T, dir, message string) string {
	t.Helper()
	id, err := w.backend.Commit(t.Context(), dir, vcs.CommitOptions{Message: message})
	mustNil(t, "commit "+message, err)
	if len(id) < 40 {
		t.Fatalf("commit %s returned %q, want a full object name", message, id)
	}
	return id
}

// upstreamCommit pushes a new commit with file=content from another clone
// and returns it.
func (w world) upstreamCommit(t *testing.T, file, content string) string {
	t.Helper()
	other := w.cloneAt(t, "other-"+filepath.Base(t.Name()))
	writeFile(t, other, file, content)
	id := w.commit(t, other, "Upstream "+file)
	mustNil(t, "push upstream", w.backend.Push(t.Context(), other, vcs.PushOptions{}))
	return id
}

func (w world) status(t *testing.T, dir string) vcs.Status {
	t.Helper()
	st, err := w.backend.Status(t.Context(), dir)
	mustNil(t, "status", err)
	return st
}

func (w world) head(t *testing.T, dir string) vcs.Head {
	t.Helper()
	h, err := w.backend.Head(t.Context(), dir)
	mustNil(t, "head", err)
	return h
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	mustNil(t, "mkdir", os.MkdirAll(filepath.Dir(path), 0o700))
	mustNil(t, "write "+name, os.WriteFile(path, []byte(content), 0o600))
}

func mustNil(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func mustIs(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: error = %v, want %v", what, err, target)
	}
}

// sortedChanges returns changes ordered by path for comparison.
func sortedChanges(changes []vcs.Change) []vcs.Change {
	out := slices.Clone(changes)
	slices.SortFunc(out, func(a, b vcs.Change) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func diffErr(w world, from, to string) error {
	_, err := w.backend.Diff(context.Background(), w.clone, from, to)
	return err
}

func remoteErr(w world, remote string) error {
	_, err := w.backend.RemoteURL(context.Background(), w.clone, remote)
	return err
}

func commitErr(w world, message string) error {
	_, err := w.backend.Commit(context.Background(), w.clone, vcs.CommitOptions{Message: message})
	return err
}

func isInvalid(err error) bool {
	return errors.Is(err, vcs.ErrInvalidArgument)
}
