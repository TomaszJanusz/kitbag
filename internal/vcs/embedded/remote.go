package embedded

import (
	"context"
	"errors"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// RemoteHead advertises upstream refs without downloading objects or updating refs.
func (b *Backend) RemoteHead(ctx context.Context, dir string) (hash string, err error) {
	r, err := open(ctx, dir)
	if err != nil {
		return "", err
	}
	h, err := head(r)
	if err != nil {
		return "", err
	}
	if h.Detached() {
		return "", vcs.ErrDetachedHead
	}
	cfg, err := r.Config()
	if err != nil {
		return "", err
	}
	branch := cfg.Branches[h.Branch]
	if branch == nil || branch.Remote == "" || branch.Merge == "" {
		return "", vcs.ErrNoUpstream
	}
	return b.advertisedHead(ctx, r, dir, branch.Remote, branch.Merge)
}

func (b *Backend) advertisedHead(ctx context.Context, r *git.Repository, dir, remoteName string, merge plumbing.ReferenceName) (hash string, err error) {
	remote, err := r.Remote(remoteName)
	if err != nil {
		return "", vcs.ErrNoRemote
	}
	url, err := b.RemoteURL(ctx, dir, remoteName)
	if err != nil {
		return "", err
	}
	auth, closeAuth, err := b.auth(ctx, r, url)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, closeAuth()) }()
	refs, err := remote.ListContext(ctx, &git.ListOptions{Auth: auth})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return "", vcs.ErrNoUpstream
	}
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if ref.Name() == merge {
			return ref.Hash().String(), nil
		}
	}
	return "", vcs.ErrNoUpstream
}
