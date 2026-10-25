package embedded

import (
	"context"
	"errors"
	"slices"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Status reports local changes and the relationship to the configured upstream.
func (*Backend) Status(ctx context.Context, dir string) (vcs.Status, error) {
	r, err := open(ctx, dir)
	if err != nil {
		return vcs.Status{}, err
	}
	h, err := head(r)
	if err != nil {
		return vcs.Status{}, err
	}
	st := vcs.Status{Head: h}
	changes, err := localStatus(ctx, r, dir)
	if err != nil {
		return st, err
	}
	for path, change := range changes {
		kind := statusKind(change)
		if kind != 0 {
			st.Changes = append(st.Changes, vcs.Change{Path: path, Kind: kind})
		}
	}
	slices.SortFunc(st.Changes, func(a, b vcs.Change) int { return strings.Compare(a.Path, b.Path) })
	if !h.Detached() {
		err = trackingStatus(ctx, r, &st)
	}
	return st, err
}

func trackingStatus(ctx context.Context, r *git.Repository, st *vcs.Status) error {
	up, err := upstream(r, st.Branch)
	if errors.Is(err, vcs.ErrNoUpstream) {
		return nil
	}
	if err != nil {
		return err
	}
	st.Upstream = up.Short()
	ref, err := r.Reference(up, true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		st.UpstreamGone = true
		return nil
	}
	if err != nil {
		return err
	}
	local, err := reachable(ctx, r, plumbing.NewHash(st.Commit))
	if err != nil {
		return err
	}
	remote, err := reachable(ctx, r, ref.Hash())
	if err != nil {
		return err
	}
	st.Ahead, st.Behind = difference(local, remote), difference(remote, local)
	return nil
}

func statusKind(s *git.FileStatus) vcs.ChangeKind {
	code := s.Worktree
	if code == git.Unmodified {
		code = s.Staging
	}
	switch code {
	case git.Unmodified:
		return 0
	case git.Untracked:
		return vcs.ChangeUntracked
	case git.Added:
		return vcs.ChangeAdded
	case git.Deleted:
		return vcs.ChangeDeleted
	case git.UpdatedButUnmerged:
		return vcs.ChangeUnmerged
	case git.Modified, git.Renamed, git.Copied:
		return vcs.ChangeModified
	}
	return vcs.ChangeModified
}

func upstream(r *git.Repository, branch string) (plumbing.ReferenceName, error) {
	cfg, err := r.Config()
	if err != nil {
		return "", err
	}
	b := cfg.Branches[branch]
	if b == nil || b.Remote == "" || b.Merge == "" {
		return "", vcs.ErrNoUpstream
	}
	if b.Remote == "." {
		return b.Merge, nil
	}
	remote := cfg.Remotes[b.Remote]
	if remote == nil {
		return "", vcs.ErrNoUpstream
	}
	for _, spec := range remote.Fetch {
		if spec.Match(b.Merge) {
			return spec.Dst(b.Merge), nil
		}
	}
	return "", vcs.ErrNoUpstream
}

func reachable(ctx context.Context, r *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := map[plumbing.Hash]bool{}
	if start.IsZero() {
		return seen, nil
	}
	shallow, err := r.Storer.Shallow()
	if err != nil {
		return nil, err
	}
	pending := []plumbing.Hash{start}
	for len(pending) > 0 {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		hash := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[hash] {
			continue
		}
		c, readErr := r.CommitObject(hash)
		if readErr != nil {
			return nil, readErr
		}
		seen[hash] = true
		if !slices.Contains(shallow, hash) {
			pending = append(pending, c.ParentHashes...)
		}
	}
	return seen, nil
}

func difference(a, b map[plumbing.Hash]bool) int {
	n := 0
	for hash := range a {
		if !b[hash] {
			n++
		}
	}
	return n
}
