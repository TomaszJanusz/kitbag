package embedded

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Diff compares committed trees without rename detection or external tools.
func (*Backend) Diff(ctx context.Context, dir, from, to string) ([]vcs.Change, error) {
	return TreeDiff(ctx, dir, from, to)
}

// Preview renders the shared unified tree diff.
func (*Backend) Preview(ctx context.Context, dir, from, to string) (string, error) {
	return TreePreview(ctx, dir, from, to)
}

const fileTypeMask = 0o170000

// TreeDiff is shared by system and embedded so paths and types are identical.
func TreeDiff(ctx context.Context, dir, from, to string) ([]vcs.Change, error) {
	changes, err := treeChanges(ctx, dir, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]vcs.Change, 0, len(changes))
	for _, change := range changes {
		action, actionErr := change.Action()
		if actionErr != nil {
			return nil, actionErr
		}
		c := vcs.Change{Path: change.To.Name, Kind: vcs.ChangeModified}
		switch action {
		case merkletrie.Insert:
			c.Kind = vcs.ChangeAdded
		case merkletrie.Delete:
			c.Kind = vcs.ChangeDeleted
			c.Path = change.From.Name
		case merkletrie.Modify:
			if change.From.TreeEntry.Mode != change.To.TreeEntry.Mode && change.From.TreeEntry.Mode&fileTypeMask != change.To.TreeEntry.Mode&fileTypeMask {
				c.Kind = vcs.ChangeTypeChanged
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// TreePreview uses go-git's tree/blob patch renderer (sergi/go-diff internally).
// Attributes, textconv, external diff programs and rename heuristics never apply.
func TreePreview(ctx context.Context, dir, from, to string) (string, error) {
	changes, err := treeChanges(ctx, dir, from, to)
	if err != nil {
		return "", err
	}
	patch, err := changes.PatchContext(ctx)
	if err != nil {
		return "", err
	}
	return patch.String(), nil
}

func treeChanges(ctx context.Context, dir, from, to string) (object.Changes, error) {
	for _, rev := range []string{from, to} {
		if err := checkName(rev); err != nil {
			return nil, err
		}
	}
	r, err := open(ctx, dir)
	if err != nil {
		return nil, err
	}
	var trees []*object.Tree
	for _, rev := range []string{from, to} {
		hash, resolveErr := r.ResolveRevision(plumbing.Revision(rev))
		if resolveErr != nil {
			return nil, resolveErr
		}
		commit, readErr := r.CommitObject(*hash)
		if readErr != nil {
			return nil, readErr
		}
		tree, treeErr := commit.Tree()
		if treeErr != nil {
			return nil, treeErr
		}
		trees = append(trees, tree)
	}
	return object.DiffTreeWithOptions(ctx, trees[0], trees[1], &object.DiffTreeOptions{DetectRenames: false})
}
