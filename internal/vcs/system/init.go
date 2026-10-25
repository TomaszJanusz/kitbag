package system

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Init creates an unborn main branch through the isolated Git runner. Refuse
// existing metadata rather than allowing git init to reconfigure a repository.
func (b *Backend) Init(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return vcs.ErrInvalidArgument
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			return fs.ErrExist
		}
		return err
	}
	_, err := b.repo(ctx, dir, "init", "--quiet", "--initial-branch=main", "--template=")
	return err
}
