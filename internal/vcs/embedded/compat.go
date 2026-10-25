package embedded

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// go-git rejects control characters even in local filenames. Read and stage
// those through a rooted filesystem, without weakening its checkout validator.
// No leading symlink is walked and reads cannot leave the working directory.
func localStatus(ctx context.Context, r *git.Repository, dir string) (git.Status, error) {
	wt, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := wt.Status()
	if err == nil || !strings.Contains(err.Error(), "contains control character") {
		return status, err
	}
	return compatibleStatus(ctx, r, dir)
}

func compatibleStatus(ctx context.Context, r *git.Repository, dir string) (git.Status, error) {
	idx, err := r.Storer.Index()
	if err != nil {
		return nil, err
	}
	before, err := headFiles(r)
	if err != nil {
		return nil, err
	}
	files, err := scanFiles(ctx, dir, idx)
	if err != nil {
		return nil, err
	}
	out := git.Status{}
	for _, e := range idx.Entries {
		code := compareEntry(before[e.Name], e)
		work := compareEntry(e, files[e.Name])
		if code != git.Unmodified || work != git.Unmodified {
			out[e.Name] = &git.FileStatus{Staging: code, Worktree: work}
		}
		delete(before, e.Name)
		delete(files, e.Name)
	}
	for name := range before {
		out[name] = &git.FileStatus{Staging: git.Deleted, Worktree: git.Unmodified}
	}
	for name := range files {
		out[name] = &git.FileStatus{Staging: git.Untracked, Worktree: git.Untracked}
	}
	return out, nil
}

func compareEntry(a, b *index.Entry) git.StatusCode {
	switch {
	case a == nil:
		return git.Added
	case b == nil:
		return git.Deleted
	case a.Hash != b.Hash || a.Mode != b.Mode:
		return git.Modified
	default:
		return git.Unmodified
	}
}

func headFiles(r *git.Repository) (map[string]*index.Entry, error) {
	entries := map[string]*index.Entry{}
	ref, err := r.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}
	files := tree.Files()
	err = files.ForEach(func(f *object.File) error {
		entries[f.Name] = &index.Entry{Name: f.Name, Hash: f.Hash, Mode: f.Mode}
		return nil
	})
	return entries, err
}

func scanFiles(ctx context.Context, dir string, idx *index.Index) (files map[string]*index.Entry, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	patterns, err := ignorePatterns(root)
	if err != nil {
		return nil, err
	}
	matcher := gitignore.NewMatcher(patterns)
	files = map[string]*index.Entry{}
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if path == ".git" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		_, trackedErr := idx.Entry(path)
		if trackedErr != nil && matcher.Match(strings.Split(path, "/"), false) {
			return nil
		}
		entry, readErr := readEntry(root, path, d)
		if readErr != nil {
			return readErr
		}
		files[path] = entry
		return nil
	})
	return files, err
}

func readEntry(root *os.Root, path string, d fs.DirEntry) (*index.Entry, error) {
	info, err := d.Info()
	if err != nil {
		return nil, err
	}
	mode, err := filemode.NewFromOSFileMode(info.Mode())
	if err != nil {
		return nil, err
	}
	data, err := entryBytes(root, path, mode)
	if err != nil {
		return nil, err
	}
	return &index.Entry{Name: path, Mode: mode, Hash: plumbing.ComputeHash(plumbing.BlobObject, data), Size: uint32(len(data)), ModifiedAt: info.ModTime()}, nil //nolint:gosec // G115: Git's index size is a uint32, modulo 2^32 as in Git.
}

func entryBytes(root *os.Root, path string, mode filemode.FileMode) ([]byte, error) {
	if mode == filemode.Symlink {
		target, err := root.Readlink(path)
		return []byte(target), err
	}
	return root.ReadFile(path)
}

func ignorePatterns(root *os.Root) ([]gitignore.Pattern, error) {
	var patterns []gitignore.Pattern
	err := fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == ".git" {
			return fs.SkipDir
		}
		if d.Name() != ".gitignore" || d.IsDir() {
			return nil
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		domain := strings.Split(path, "/")
		domain = domain[:len(domain)-1]
		for line := range strings.SplitSeq(string(data), "\n") {
			patterns = append(patterns, gitignore.ParsePattern(line, domain))
		}
		return nil
	})
	return patterns, err
}

func stageAll(ctx context.Context, r *git.Repository, dir string) error {
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	err = wt.AddWithOptions(&git.AddOptions{All: true})
	if err == nil || !strings.Contains(err.Error(), "contains control character") {
		return err
	}
	return compatibleStage(ctx, r, dir)
}

func compatibleStage(ctx context.Context, r *git.Repository, dir string) (err error) {
	idx, err := r.Storer.Index()
	if err != nil {
		return err
	}
	files, err := scanFiles(ctx, dir, idx)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	entries := make([]*index.Entry, 0, len(files))
	for _, entry := range files {
		data, readErr := entryBytes(root, entry.Name, entry.Mode)
		if readErr != nil {
			return readErr
		}
		obj := &plumbing.MemoryObject{}
		obj.SetType(plumbing.BlobObject)
		writer, writeErr := obj.Writer()
		if writeErr != nil {
			return writeErr
		}
		_, writeErr = writer.Write(data)
		if writeErr = errors.Join(writeErr, writer.Close()); writeErr != nil {
			return writeErr
		}
		entry.Hash, writeErr = r.Storer.SetEncodedObject(obj)
		if writeErr != nil {
			return writeErr
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b *index.Entry) int { return strings.Compare(a.Name, b.Name) })
	idx.Entries = entries
	return r.Storer.SetIndex(idx)
}
