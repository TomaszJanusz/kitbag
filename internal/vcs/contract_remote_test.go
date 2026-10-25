package vcs_test

import (
	"errors"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (s suite) testRemoteHead(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	before := w.status(t, w.clone)
	initial, err := w.backend.RemoteHead(t.Context(), w.clone)
	mustNil(t, "remote head", err)
	if initial != before.Commit {
		t.Fatalf("remote=%s local=%s", initial, before.Commit)
	}
	pushed := w.upstreamCommit(t, "changed", "new")
	remote, err := w.backend.RemoteHead(t.Context(), w.clone)
	mustNil(t, "moved remote", err)
	if remote != pushed {
		t.Fatalf("remote=%s want=%s", remote, pushed)
	}
	after := w.status(t, w.clone)
	if after.Commit != before.Commit || after.Behind != before.Behind {
		t.Fatal("probe fetched or updated local state")
	}
	w.git(t, w.remote, "update-ref", "-d", "refs/heads/main")
	_, err = w.backend.RemoteHead(t.Context(), w.clone)
	if !errors.Is(err, vcs.ErrNoUpstream) {
		t.Fatalf("deleted upstream=%v", err)
	}
	w.git(t, w.clone, "checkout", "--detach", "--quiet")
	_, err = w.backend.RemoteHead(t.Context(), w.clone)
	mustIs(t, "detached", err, vcs.ErrDetachedHead)
}
