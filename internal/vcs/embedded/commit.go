package embedded

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Commit stages all changes and records an unsigned commit with explicit identity.
func (b *Backend) Commit(ctx context.Context, dir string, opts vcs.CommitOptions) (string, error) {
	if strings.TrimSpace(opts.Message) == "" {
		return "", vcs.ErrInvalidArgument
	}
	r, wt, author, err := b.prepareCommit(ctx, dir)
	if err != nil {
		return "", err
	}
	st, err := localStatus(ctx, r, dir)
	if err != nil {
		return "", err
	}
	if st.IsClean() {
		return "", vcs.ErrNothingToCommit
	}
	if err = stageAll(ctx, r, dir); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	hash, err := wt.Commit(opts.Message, &git.CommitOptions{Author: author})
	if errors.Is(err, git.ErrEmptyCommit) {
		return "", vcs.ErrNothingToCommit
	}
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return hash.String(), nil
}

func (b *Backend) prepareCommit(ctx context.Context, dir string) (*git.Repository, *git.Worktree, *object.Signature, error) {
	r, err := open(ctx, dir)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = checkInProgress(dir); err != nil {
		return nil, nil, nil, err
	}
	configs, err := b.configs(r)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = checkConfigs(configs, true); err != nil {
		return nil, nil, nil, err
	}
	id, err := b.identity(configs)
	if err != nil {
		return nil, nil, nil, err
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, nil, nil, err
	}
	return r, wt, &object.Signature{Name: id.Name, Email: id.Email, When: time.Now()}, nil
}
