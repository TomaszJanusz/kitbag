package embedded

import (
	"context"
	"errors"
	"fmt"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// FastForward updates only a clean branch whose upstream is a descendant.
func (b *Backend) FastForward(ctx context.Context, dir string) error {
	st, err := b.Status(ctx, dir)
	if err != nil {
		return err
	}
	if err = canFastForward(st); err != nil {
		return err
	}
	if st.Behind == 0 {
		return nil
	}
	if !st.Clean() {
		return errors.New("fast-forward: working copy has local changes")
	}
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	if err = b.checkUpdate(r, dir); err != nil {
		return err
	}
	return resetToUpstream(r, st)
}

func resetToUpstream(r *git.Repository, st vcs.Status) error {
	up, err := upstream(r, st.Branch)
	if err != nil {
		return err
	}
	ref, err := r.Reference(up, true)
	if err != nil {
		return err
	}
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	mode := git.MergeReset
	if st.Unborn() {
		return wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(st.Branch), Hash: ref.Hash(), Create: true})
	}
	return wt.Reset(&git.ResetOptions{Commit: ref.Hash(), Mode: mode})
}

func canFastForward(st vcs.Status) error {
	switch {
	case st.Detached():
		return vcs.ErrDetachedHead
	case st.Upstream == "" || st.UpstreamGone:
		return vcs.ErrNoUpstream
	case st.Ahead > 0 && st.Behind > 0:
		return &vcs.UnsupportedError{Feature: "non-fast-forward merge", Err: vcs.ErrNotFastForward}
	}
	return nil
}

// DropCommit moves a clean branch from its unpublished HEAD commit back to
// that commit's only parent.
func (b *Backend) DropCommit(ctx context.Context, dir, commit string) error {
	st, err := b.Status(ctx, dir)
	if err != nil {
		return err
	}
	if err = vcs.CheckDrop(st, commit); err != nil {
		return err
	}
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	if err = b.checkUpdate(r, dir); err != nil {
		return err
	}
	hash := plumbing.NewHash(commit)
	if err = unpublished(ctx, r, st.Branch, hash); err != nil {
		return err
	}
	c, err := r.CommitObject(hash)
	if err != nil {
		return err
	}
	if c.NumParents() != 1 {
		return fmt.Errorf("drop commit %s: not a single-parent commit: %w", commit, vcs.ErrInvalidArgument)
	}
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	return wt.Reset(&git.ResetOptions{Commit: c.ParentHashes[0], Mode: git.MergeReset})
}

// unpublished refuses a commit reachable from branch's upstream.
func unpublished(ctx context.Context, r *git.Repository, branch string, hash plumbing.Hash) error {
	up, err := upstream(r, branch)
	if err != nil {
		return err
	}
	ref, err := r.Reference(up, true)
	if err != nil {
		return err
	}
	published, err := reachable(ctx, r, ref.Hash())
	if err != nil {
		return err
	}
	if published[hash] {
		return fmt.Errorf("drop commit %s: %w", hash, vcs.ErrPublished)
	}
	return nil
}

// CreateBranch creates a local branch without checking it out or tracking.
func (*Backend) CreateBranch(ctx context.Context, dir, name, start string) error {
	ref, err := branchName(name)
	if err != nil {
		return err
	}
	if start == "" {
		start = "HEAD"
	}
	if err = checkName(start); err != nil {
		return err
	}
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	_, err = r.Reference(ref, false)
	if err == nil {
		return fmt.Errorf("branch %s already exists", name)
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return err
	}
	hash, err := r.ResolveRevision(plumbing.Revision(start))
	if err != nil {
		return err
	}
	return r.Storer.SetReference(plumbing.NewHashReference(ref, *hash))
}

// DeleteBranch removes a non-current branch only if it is fully merged.
func (*Backend) DeleteBranch(ctx context.Context, dir, name string) error {
	ref, err := branchName(name)
	if err != nil {
		return err
	}
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	h, err := head(r)
	if err != nil {
		return err
	}
	if h.Branch == name {
		return fmt.Errorf("cannot delete current branch %s", name)
	}
	target, err := r.Reference(ref, true)
	if err != nil {
		return err
	}
	merged, err := reachable(ctx, r, plumbing.NewHash(h.Commit))
	if err != nil {
		return err
	}
	if !merged[target.Hash()] {
		return fmt.Errorf("branch %s is not fully merged", name)
	}
	if err = r.Storer.RemoveReference(ref); err != nil {
		return err
	}
	cfg, err := r.Config()
	if err != nil {
		return err
	}
	delete(cfg.Branches, name)
	return r.SetConfig(cfg)
}

func (b *Backend) checkUpdate(r *git.Repository, dir string) error {
	configs, err := b.configs(r)
	if err != nil {
		return err
	}
	if err = checkConfigs(configs, false); err != nil {
		return err
	}
	return checkInProgress(dir)
}
