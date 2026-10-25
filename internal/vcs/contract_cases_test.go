package vcs_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Paths that break line- or space-based parsing, or quoting.
const (
	spacePath   = "dir with space/a b.txt"
	newlinePath = "new\nline.txt"
	unicodePath = "z-ünïcödé ✓.txt"
)

func (s suite) testCloneEmptyIsUnborn(t *testing.T) {
	t.Parallel()
	sub := s.newSubject(t)
	root := t.TempDir()
	remote, dir := filepath.Join(root, "empty.git"), filepath.Join(root, "clone")
	sub.git(t, root, "init", "--quiet", "--bare", "--initial-branch=main", remote)
	mustNil(t, "clone", sub.backend.Clone(t.Context(), remote, dir, vcs.CloneOptions{}))

	h, err := sub.backend.Head(t.Context(), dir)
	mustNil(t, "head", err)
	if h != (vcs.Head{Branch: "main"}) || !h.Unborn() || h.Detached() {
		t.Errorf("head = %+v, want unborn main", h)
	}
	st, err := sub.backend.Status(t.Context(), dir)
	mustNil(t, "status", err)
	// The empty remote has no main branch yet, so the upstream is gone.
	if !st.Clean() || !st.Unborn() || st.Upstream != "origin/main" || st.Behind != 0 || !st.UpstreamGone {
		t.Errorf("status = %+v, want clean unborn main tracking a missing origin/main", st)
	}
}

func (s suite) testCommitPushClone(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	writeFile(t, w.clone, "added.txt", "new\n")
	id := w.commit(t, w.clone, "Add")
	if h := w.head(t, w.clone); h.Commit != id || h.Branch != "main" {
		t.Errorf("head = %+v, want main at %s", h, id)
	}
	if st := w.status(t, w.clone); st.Ahead != 1 || st.Behind != 0 || !st.Clean() {
		t.Errorf("status before push = %+v, want clean and one ahead", st)
	}
	mustNil(t, "push", w.backend.Push(t.Context(), w.clone, vcs.PushOptions{}))
	if st := w.status(t, w.clone); st.Ahead != 0 || st.Behind != 0 {
		t.Errorf("status after push = %+v, want even with upstream", st)
	}

	fresh := w.cloneAt(t, "fresh")
	if h := w.head(t, fresh); h.Commit != id {
		t.Errorf("fresh clone head = %+v, want %s", h, id)
	}
	if _, err := os.Stat(filepath.Join(fresh, "added.txt")); err != nil {
		t.Errorf("fresh clone lacks the pushed file: %v", err)
	}
}

func (s suite) testNothingToCommit(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	before := w.head(t, w.clone)
	_, err := w.backend.Commit(t.Context(), w.clone, vcs.CommitOptions{Message: "Empty"})
	mustIs(t, "commit without changes", err, vcs.ErrNothingToCommit)
	if after := w.head(t, w.clone); after != before {
		t.Errorf("head moved from %+v to %+v", before, after)
	}
}

func (s suite) testStatusChanges(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	writeFile(t, w.clone, spacePath, "x\n")
	writeFile(t, w.clone, newlinePath, "x\n")
	writeFile(t, w.clone, unicodePath, "x\n")
	writeFile(t, w.clone, "seed.txt", "changed\n")
	w.commit(t, w.clone, "Tracked")

	writeFile(t, w.clone, spacePath, "modified\n")
	mustNil(t, "remove", os.Remove(filepath.Join(w.clone, filepath.FromSlash(unicodePath))))
	writeFile(t, w.clone, "untracked dir/new\nfile ✓", "u\n")

	got := sortedChanges(w.status(t, w.clone).Changes)
	want := []vcs.Change{
		{Path: spacePath, Kind: vcs.ChangeModified},
		{Path: "untracked dir/new\nfile ✓", Kind: vcs.ChangeUntracked},
		{Path: unicodePath, Kind: vcs.ChangeDeleted},
	}
	if !slices.Equal(got, want) {
		t.Errorf("changes = %q, want %q", got, want)
	}
}

func (s suite) testDiffOfTrees(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	from := w.head(t, w.clone).Commit
	writeFile(t, w.clone, spacePath, "a\n")
	writeFile(t, w.clone, newlinePath, "b\n")
	writeFile(t, w.clone, "seed.txt", "changed\n")
	to := w.commit(t, w.clone, "Change")
	mustNil(t, "remove", os.Remove(filepath.Join(w.clone, filepath.FromSlash(spacePath))))
	writeFile(t, w.clone, unicodePath, "c\n")
	last := w.commit(t, w.clone, "Again")

	cases := []struct {
		from, to string
		want     []vcs.Change
	}{
		{from, to, []vcs.Change{
			{Path: spacePath, Kind: vcs.ChangeAdded},
			{Path: newlinePath, Kind: vcs.ChangeAdded},
			{Path: "seed.txt", Kind: vcs.ChangeModified},
		}},
		{to, last, []vcs.Change{
			{Path: spacePath, Kind: vcs.ChangeDeleted},
			{Path: unicodePath, Kind: vcs.ChangeAdded},
		}},
		{last, last, nil},
	}
	for _, tc := range cases {
		got, err := w.backend.Diff(t.Context(), w.clone, tc.from, tc.to)
		mustNil(t, "diff", err)
		if got = sortedChanges(got); !slices.Equal(got, sortedChanges(tc.want)) {
			t.Errorf("diff %.7s..%.7s = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}
}

func (s suite) testPullFastForwards(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	id := w.upstreamCommit(t, "up.txt", "up\n")

	res, err := vcs.Pull(t.Context(), w.backend, w.clone)
	mustNil(t, "pull", err)
	if res.Outcome != vcs.PullUpdated || res.Status.Commit != id || res.Status.Behind != 0 {
		t.Errorf("pull = %v %+v, want updated to %s", res.Outcome, res.Status, id)
	}
	if _, statErr := os.Stat(filepath.Join(w.clone, "up.txt")); statErr != nil {
		t.Errorf("working tree lacks the upstream file: %v", statErr)
	}

	res, err = vcs.Pull(t.Context(), w.backend, w.clone)
	mustNil(t, "second pull", err)
	if res.Outcome != vcs.PullUpToDate {
		t.Errorf("second pull = %v, want up to date", res.Outcome)
	}
}

func (s suite) testPullIntoUnborn(t *testing.T) {
	t.Parallel()
	sub := s.newSubject(t)
	root := t.TempDir()
	w := world{subject: sub, remote: filepath.Join(root, "remote.git"), clone: filepath.Join(root, "clone")}
	sub.git(t, root, "init", "--quiet", "--bare", "--initial-branch=main", w.remote)
	mustNil(t, "clone empty", sub.backend.Clone(t.Context(), w.remote, w.clone, vcs.CloneOptions{}))
	id := w.upstreamCommit(t, "first.txt", "first\n")

	res, err := vcs.Pull(t.Context(), sub.backend, w.clone)
	mustNil(t, "pull", err)
	if res.Outcome != vcs.PullUpdated || res.Status.Commit != id {
		t.Errorf("pull = %v %+v, want updated to %s", res.Outcome, res.Status, id)
	}
}

func (s suite) testPullSkipsDirty(t *testing.T) {
	t.Parallel()
	for name, dirty := range map[string]func(t *testing.T, dir string){
		"modified":  func(t *testing.T, dir string) { t.Helper(); writeFile(t, dir, "seed.txt", "local\n") },
		"untracked": func(t *testing.T, dir string) { t.Helper(); writeFile(t, dir, "local.txt", "local\n") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := s.newWorld(t)
			before := w.head(t, w.clone)
			w.upstreamCommit(t, "up.txt", "up\n")
			dirty(t, w.clone)

			res, err := vcs.Pull(t.Context(), w.backend, w.clone)
			mustNil(t, "pull", err)
			if res.Outcome != vcs.PullSkippedDirty {
				t.Errorf("pull = %v, want skipped", res.Outcome)
			}
			// Nothing was fetched: the remote-tracking branch is unchanged.
			if st := w.status(t, w.clone); st.Commit != before.Commit || st.Behind != 0 {
				t.Errorf("status = %+v, want untouched at %s", st, before.Commit)
			}
		})
	}
}

func (s suite) testPullLeavesDiverged(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	w.upstreamCommit(t, "up.txt", "up\n")
	writeFile(t, w.clone, "local.txt", "local\n")
	local := w.commit(t, w.clone, "Local")

	res, err := vcs.Pull(t.Context(), w.backend, w.clone)
	mustNil(t, "pull", err)
	if res.Outcome != vcs.PullDiverged || res.Status.Ahead != 1 || res.Status.Behind != 1 {
		t.Errorf("pull = %v %+v, want diverged 1/1", res.Outcome, res.Status)
	}
	mustIs(t, "fast-forward", w.backend.FastForward(t.Context(), w.clone), vcs.ErrNotFastForward)
	if h := w.head(t, w.clone); h.Commit != local {
		t.Errorf("head = %+v, want unchanged %s", h, local)
	}
}

func (s suite) testPushRejectsNonFastForward(t *testing.T) {
	t.Parallel()
	w := s.newWorld(t)
	upstream := w.upstreamCommit(t, "up.txt", "up\n")
	writeFile(t, w.clone, "local.txt", "local\n")
	w.commit(t, w.clone, "Local")

	err := w.backend.Push(t.Context(), w.clone, vcs.PushOptions{})
	mustIs(t, "push", err, vcs.ErrPushRejected)
	if h := w.head(t, w.cloneAt(t, "check")); h.Commit != upstream {
		t.Errorf("remote main = %s, want unchanged %s", h.Commit, upstream)
	}
}
