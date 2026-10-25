package embedded

import (
	"context"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Init creates an unborn main branch without starting system Git.
func (*Backend) Init(ctx context.Context, dir string) error {
	if err := checkDir(ctx, dir); err != nil {
		return err
	}
	_, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: plumbing.Main}})
	return err
}

// InspectCommit checks effective identity without committing or disclosing it.
func (b *Backend) InspectCommit(ctx context.Context, dir string) (vcs.CommitConfig, error) {
	r, err := open(ctx, dir)
	if err != nil {
		return vcs.CommitConfig{}, err
	}
	configs, err := b.configs(r)
	if err != nil {
		return vcs.CommitConfig{}, err
	}
	_, identityErr := b.identity(configs)
	return vcs.CommitConfig{Identity: identityErr == nil}, nil
}
