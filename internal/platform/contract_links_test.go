package platform_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s suite) runLinks(t *testing.T) {
	t.Helper()
	t.Run("Link/StoresTargetVerbatim", s.linkStoresTargetVerbatim)
	t.Run("Link/ReplacesFilesAndLinks", s.linkReplacesFilesAndLinks)
	t.Run("Link/RefusesDirectories", s.linkRefusesDirectories)
	t.Run("Link/Errors", s.linkErrors)
	t.Run("ReadLink/Errors", s.readLinkErrors)
	t.Run("IsManagedLink/Targets", s.managedLinkTargets)
	t.Run("IsManagedLink/OtherEntries", s.managedLinkOtherEntries)
}

func (s suite) linkStoresTargetVerbatim(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	for name, target := range map[string]string{
		"absolute": filepath.Join(sub.root, "repo", "skill"),
		"relative": filepath.Join("..", "repo", "skill"),
		"dangling": filepath.Join(sub.root, "missing"),
	} {
		path := filepath.Join(sub.root, name)
		mustLink(t, sub.host, target, path)
		wantKind(t, sub.host, path, fs.ModeSymlink, 0)
		if got, err := sub.host.ReadLink(path); err != nil || got != target {
			t.Errorf("ReadLink(%s) = %q, %v; want %q", name, got, err, target)
		}
	}
	wantNames(t, sub.host, sub.root, "absolute", "dangling", "relative")
}

func (s suite) linkReplacesFilesAndLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	first, second := filepath.Join(sub.root, "first"), filepath.Join(sub.root, "second")
	mustWrite(t, h, first, "first", 0o644)
	mustWrite(t, h, second, "second", 0o644)
	path := filepath.Join(sub.root, "target")
	mustWrite(t, h, path, "unmanaged", 0o600)

	mustLink(t, h, first, path)
	wantContent(t, h, path, "first")
	mustLink(t, h, second, path)
	wantContent(t, h, path, "second")

	wantContent(t, h, first, "first")
	wantKind(t, h, first, 0, 0o644)
	wantNames(t, h, sub.root, "first", "second", "target")
}

func (s suite) linkRefusesDirectories(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	dir := filepath.Join(sub.root, "dir")
	mustWrite(t, h, filepath.Join(sub.root, "file"), "x", 0o644)
	mustMkdir(t, h, dir)
	mustWrite(t, h, filepath.Join(dir, "keep"), "keep", 0o644)

	wantErr(t, "Link over directory", h.Link(filepath.Join(sub.root, "file"), dir), platform.ErrIsDir)
	wantKind(t, h, dir, fs.ModeDir, 0o755)
	wantContent(t, h, filepath.Join(dir, "keep"), "keep")

	// A link to a directory is a link: replacing it leaves the directory alone.
	toDir := filepath.Join(sub.root, "to-dir")
	mustLink(t, h, dir, toDir)
	mustLink(t, h, filepath.Join(sub.root, "file"), toDir)
	wantContent(t, h, toDir, "x")
	wantContent(t, h, filepath.Join(dir, "keep"), "keep")
	wantNames(t, h, sub.root, "dir", "file", "to-dir")
}

func (s suite) linkErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	wantErr(t, "missing parent", h.Link(file, filepath.Join(sub.root, "missing", "l")), fs.ErrNotExist)
	wantErr(t, "file parent", h.Link(file, filepath.Join(file, "l")), platform.ErrNotDir)
	wantErr(t, "empty target", h.Link("", filepath.Join(sub.root, "l")), fs.ErrInvalid)
	wantNames(t, h, sub.root, "file")
}

func (s suite) readLinkErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	mustWrite(t, h, filepath.Join(sub.root, "file"), "x", 0o644)
	mustMkdir(t, h, filepath.Join(sub.root, "dir"))
	for name, want := range map[string]error{"file": platform.ErrNotLink, "dir": platform.ErrNotLink, "missing": fs.ErrNotExist} {
		_, err := h.ReadLink(filepath.Join(sub.root, name))
		wantErr(t, "ReadLink "+name, err, want)
	}
}

func (s suite) managedLinkTargets(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	repo, old := filepath.Join(sub.root, "repo"), filepath.Join(sub.root, "old-clone")
	links := filepath.Join(sub.root, "agent", "skills")
	mustMkdir(t, sub.host, links)
	for _, tc := range []struct {
		target string
		want   bool
	}{
		{target: filepath.Join(repo, "skills", "a"), want: true},
		{target: repo, want: true},
		{target: filepath.Join(old, "b"), want: true},
		{target: filepath.Join("..", "..", "repo", "skills", "c"), want: true},
		{target: filepath.Join(repo, "missing"), want: true},
		{target: sub.root + string(filepath.Separator) + "repo-other", want: false},
		{target: filepath.Join(sub.root, "re"), want: false},
		{target: filepath.Join("..", "repo"), want: false},
		{target: raw(repo, "..", "elsewhere"), want: false},
		{target: raw("..", "..", "repo", "..", "x"), want: false},
		{target: raw("..", "..", "repo", "skills", "..", "d"), want: true},
	} {
		path := filepath.Join(links, "l")
		mustLink(t, sub.host, tc.target, path)
		if got, err := sub.host.IsManagedLink(path, repo, old); err != nil || got != tc.want {
			t.Errorf("IsManagedLink(-> %s) = %v, %v; want %v", tc.target, got, err, tc.want)
		}
	}
}

func (s suite) managedLinkOtherEntries(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, repo := sub.host, filepath.Join(sub.root, "repo")
	mustMkdir(t, h, repo)
	mustWrite(t, h, filepath.Join(repo, "file"), "x", 0o644)
	link := filepath.Join(sub.root, "link")
	mustLink(t, h, filepath.Join(repo, "file"), link)
	for _, path := range []string{repo, filepath.Join(repo, "file")} {
		if got, err := h.IsManagedLink(path, sub.root); err != nil || got {
			t.Errorf("IsManagedLink(%s) = %v, %v; want false, nil", path, got, err)
		}
	}
	if got, err := h.IsManagedLink(link); err != nil || got {
		t.Errorf("IsManagedLink without roots = %v, %v; want false, nil", got, err)
	}
	_, err := h.IsManagedLink(filepath.Join(sub.root, "missing"), repo)
	wantErr(t, "missing", err, fs.ErrNotExist)
	_, err = h.IsManagedLink(link, "repo")
	wantErr(t, "relative root", err, fs.ErrInvalid)
}
