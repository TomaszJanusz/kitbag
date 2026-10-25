package system

import (
	"context"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// RemoteHead lists only the configured upstream ref; it never fetches objects.
func (b *Backend) RemoteHead(ctx context.Context, dir string) (string, error) {
	h, err := b.Head(ctx, dir)
	if err != nil {
		return "", err
	}
	if h.Detached() {
		return "", vcs.ErrDetachedHead
	}
	remote, err := b.optional(ctx, dir, "config", "--get", "branch."+h.Branch+".remote")
	if err != nil {
		return "", err
	}
	ref, err := b.optional(ctx, dir, "config", "--get", "branch."+h.Branch+".merge")
	if err != nil {
		return "", err
	}
	if remote == "" || ref == "" {
		return "", vcs.ErrNoUpstream
	}
	if err = checkName("remote", remote); err != nil {
		return "", err
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		return "", vcs.ErrInvalidArgument
	}
	return b.advertisedHead(ctx, dir, remote, ref)
}

func (b *Backend) advertisedHead(ctx context.Context, dir, remote, ref string) (string, error) {
	res, err := b.repo(ctx, dir, "ls-remote", "--refs", "--", remote, ref)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(res.Stdout), "\n") {
		hash, name, found := strings.Cut(line, "\t")
		if found && name == ref {
			return hash, nil
		}
	}
	return "", vcs.ErrNoUpstream
}
