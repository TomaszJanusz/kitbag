package embedded

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Clone clones a full or shallow repository without starting Git processes.
func (b *Backend) Clone(ctx context.Context, url, dir string, opts vcs.CloneOptions) (err error) {
	if err = checkDir(ctx, dir); err != nil {
		return err
	}
	cloneOpts, err := cloneOptions(url, opts)
	if err != nil {
		return err
	}
	auth, closeAuth, err := b.auth(ctx, nil, url)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeAuth()) }()
	cloneOpts.Auth = auth
	return b.cloneRepository(ctx, dir, cloneOpts)
}

func cloneOptions(url string, opts vcs.CloneOptions) (*git.CloneOptions, error) {
	if err := checkName(url); err != nil {
		return nil, err
	}
	if opts.Depth < 0 {
		return nil, vcs.ErrInvalidArgument
	}
	cloneOpts := &git.CloneOptions{URL: url, RemoteName: vcs.DefaultRemote, Depth: opts.Depth}
	if opts.Branch != "" {
		ref, err := branchName(opts.Branch)
		if err != nil {
			return nil, err
		}
		cloneOpts.ReferenceName = ref
	}
	return cloneOpts, nil
}

func (b *Backend) cloneRepository(ctx context.Context, dir string, opts *git.CloneOptions) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return errors.New("clone destination is not empty")
	}
	_, err := git.PlainCloneContext(ctx, dir, false, opts)
	if !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return err
	}
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}
	if _, err = r.CreateRemote(&config.RemoteConfig{Name: vcs.DefaultRemote, URLs: []string{opts.URL}}); err != nil {
		return err
	}
	return b.emptyClone(r, opts.URL, opts.ReferenceName.Short())
}

func (b *Backend) emptyClone(r *git.Repository, url, branch string) error {
	if branch == "" {
		branch = "main"
	}
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return err
	}
	if ep.Protocol == "file" {
		remote, readErr := git.PlainOpen(ep.Path)
		if readErr != nil {
			return readErr
		}
		ref, readErr := remote.Reference(plumbing.HEAD, false)
		if readErr != nil {
			return readErr
		}
		if ref.Type() == plumbing.SymbolicReference {
			branch = ref.Target().Short()
		}
	}
	if err = r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))); err != nil {
		return err
	}
	return setUpstream(r, branch)
}

// Fetch updates origin's remote-tracking refs without changing the worktree.
func (b *Backend) Fetch(ctx context.Context, dir string) (err error) {
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	url, err := b.RemoteURL(ctx, dir, vcs.DefaultRemote)
	if err != nil {
		return err
	}
	auth, closeAuth, err := b.auth(ctx, r, url)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeAuth()) }()
	err = r.FetchContext(ctx, &git.FetchOptions{RemoteName: vcs.DefaultRemote, Auth: auth, Prune: true})
	if errors.Is(err, git.NoErrAlreadyUpToDate) || errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil
	}
	return err
}

// Push sends an explicit branch to origin, rejecting non-fast-forward updates.
func (b *Backend) Push(ctx context.Context, dir string, opts vcs.PushOptions) (err error) {
	r, err := open(ctx, dir)
	if err != nil {
		return err
	}
	branch, ref, err := pushBranch(r, opts.Branch)
	if err != nil {
		return err
	}
	url, err := b.RemoteURL(ctx, dir, vcs.DefaultRemote)
	if err != nil {
		return err
	}
	auth, closeAuth, err := b.auth(ctx, r, url)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeAuth()) }()
	err = r.PushContext(ctx, &git.PushOptions{RemoteName: vcs.DefaultRemote, Auth: auth, RefSpecs: []config.RefSpec{config.RefSpec(ref + ":" + ref)}})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		err = nil
	}
	if pushRejected(err) {
		return fmt.Errorf("%w: %w", vcs.ErrPushRejected, err)
	}
	if err != nil {
		return err
	}
	if opts.SetUpstream {
		return setUpstream(r, branch)
	}
	return nil
}

// pushRejected matches refusals that go-git reports only as text: its own
// non-fast-forward check, and any per-ref status from the receiving side
// (non-fast-forward, fetch first, hook declined), like system Git's "!" lines.
func pushRejected(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "non-fast-forward update: ") || strings.HasPrefix(msg, "command error on ")
}

func setUpstream(r *git.Repository, branch string) error {
	cfg, err := r.Config()
	if err != nil {
		return err
	}
	cfg.Branches[branch] = &config.Branch{Name: branch, Remote: vcs.DefaultRemote, Merge: plumbing.NewBranchReferenceName(branch)}
	return r.SetConfig(cfg)
}

func pushBranch(r *git.Repository, branch string) (string, plumbing.ReferenceName, error) {
	if branch == "" {
		h, err := head(r)
		if err != nil {
			return "", "", err
		}
		if h.Detached() {
			return "", "", vcs.ErrDetachedHead
		}
		branch = h.Branch
	}
	ref, err := branchName(branch)
	return branch, ref, err
}
