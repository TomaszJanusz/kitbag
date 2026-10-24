package platform_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s suite) runDirectories(t *testing.T) {
	t.Helper()
	t.Run("MkdirAll/ExactMode", s.mkdirAllExactMode)
	t.Run("MkdirAll/KeepsExisting", s.mkdirAllKeepsExisting)
	t.Run("MkdirAll/ThroughLinks", s.mkdirAllThroughLinks)
	t.Run("MkdirAll/Errors", s.mkdirAllErrors)
	t.Run("Chmod/SetsExactMode", s.chmodSetsExactMode)
	t.Run("Chmod/RefusesLinks", s.chmodRefusesLinks)
}

// mkdirAllExactMode uses 0777, which any common umask would reduce.
func (s suite) mkdirAllExactMode(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	deep := filepath.Join(sub.root, "a", "b", "c")
	mustNil(t, "MkdirAll 0777", h.MkdirAll(deep, 0o777))
	for _, dir := range []string{filepath.Join(sub.root, "a"), filepath.Join(sub.root, "a", "b"), deep} {
		wantKind(t, h, dir, fs.ModeDir, 0o777)
	}
	private := filepath.Join(sub.root, "private")
	mustNil(t, "MkdirAll 0700", h.MkdirAll(private, 0o700))
	wantKind(t, h, private, fs.ModeDir, 0o700)
}

func (s suite) mkdirAllKeepsExisting(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, dir := sub.host, filepath.Join(sub.root, "existing")
	mustNil(t, "MkdirAll", h.MkdirAll(dir, 0o700))
	mustNil(t, "MkdirAll again", h.MkdirAll(dir, 0o755))
	mustNil(t, "MkdirAll child", h.MkdirAll(filepath.Join(dir, "child"), 0o750))
	wantKind(t, h, dir, fs.ModeDir, 0o700)
	wantKind(t, h, filepath.Join(dir, "child"), fs.ModeDir, 0o750)
	mustNil(t, "MkdirAll root", h.MkdirAll(sub.root, 0o755))
}

func (s suite) mkdirAllThroughLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	dir, alias := filepath.Join(sub.root, "dir"), filepath.Join(sub.root, "alias")
	mustMkdir(t, h, dir)
	mustLink(t, h, dir, alias)
	mustNil(t, "MkdirAll link", h.MkdirAll(alias, 0o700))
	mustNil(t, "MkdirAll below link", h.MkdirAll(filepath.Join(alias, "x", "y"), 0o750))
	wantKind(t, h, alias, fs.ModeSymlink, 0)
	wantKind(t, h, filepath.Join(dir, "x", "y"), fs.ModeDir, 0o750)
	wantKind(t, h, dir, fs.ModeDir, 0o755)
}

func (s suite) mkdirAllErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	dangling := filepath.Join(sub.root, "dangling")
	mustLink(t, h, filepath.Join(sub.root, "missing", "target"), dangling)
	toFile := filepath.Join(sub.root, "to-file")
	mustLink(t, h, file, toFile)
	wantErr(t, "MkdirAll(file)", h.MkdirAll(file, 0o755), platform.ErrNotDir)
	wantErr(t, "MkdirAll(file/x)", h.MkdirAll(filepath.Join(file, "x"), 0o755), platform.ErrNotDir)
	wantErr(t, "MkdirAll(link to file)", h.MkdirAll(toFile, 0o755), platform.ErrNotDir)
	wantErr(t, "MkdirAll(dangling link)", h.MkdirAll(dangling, 0o755), fs.ErrExist)
	wantNames(t, h, sub.root, "dangling", "file", "to-file")
}

func (s suite) chmodSetsExactMode(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	file, dir := filepath.Join(sub.root, "file"), filepath.Join(sub.root, "dir")
	mustWrite(t, h, file, "x", 0o644)
	mustMkdir(t, h, dir)
	mustNil(t, "Chmod file", h.Chmod(file, 0o751))
	mustNil(t, "Chmod dir", h.Chmod(dir, 0o711))
	wantKind(t, h, file, 0, 0o751)
	wantKind(t, h, dir, fs.ModeDir, 0o711)
	wantContent(t, h, file, "x")
}

func (s suite) chmodRefusesLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	file, link := filepath.Join(sub.root, "file"), filepath.Join(sub.root, "link")
	mustWrite(t, h, file, "x", 0o644)
	mustLink(t, h, file, link)
	wantErr(t, "Chmod(link)", h.Chmod(link, 0o600), fs.ErrInvalid)
	wantKind(t, h, file, 0, 0o644)
	wantErr(t, "Chmod(missing)", h.Chmod(filepath.Join(sub.root, "missing"), 0o600), fs.ErrNotExist)
}

func (s suite) runRenameRemove(t *testing.T) {
	t.Helper()
	t.Run("Rename/MovesEntries", s.renameMovesEntries)
	t.Run("Rename/RefusesExistingDestination", s.renameRefusesExisting)
	t.Run("Rename/Errors", s.renameErrors)
	t.Run("Rename/CrossDevice", s.renameCrossDevice)
	t.Run("Remove/Entries", s.removeEntries)
	t.Run("Remove/Errors", s.removeErrors)
}

func (s suite) renameMovesEntries(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	file, dir, target := filepath.Join(sub.root, "file"), filepath.Join(sub.root, "dir"), filepath.Join(sub.root, "target")
	mustWrite(t, h, file, "file", 0o640)
	mustMkdir(t, h, filepath.Join(dir, "sub"))
	mustWrite(t, h, filepath.Join(dir, "sub", "child"), "child", 0o644)
	mustWrite(t, h, target, "target", 0o644)
	mustLink(t, h, target, filepath.Join(sub.root, "link"))

	mustNil(t, "rename file", h.Rename(file, file+".before-kitbag"))
	mustNil(t, "rename dir", h.Rename(dir, filepath.Join(sub.root, "moved")))
	mustNil(t, "rename link", h.Rename(filepath.Join(sub.root, "link"), filepath.Join(sub.root, "moved", "link")))

	wantKind(t, h, file+".before-kitbag", 0, 0o640)
	wantContent(t, h, filepath.Join(sub.root, "moved", "sub", "child"), "child")
	if got, err := h.ReadLink(filepath.Join(sub.root, "moved", "link")); err != nil || got != target {
		t.Errorf("moved link = %q, %v; want %q", got, err, target)
	}
	wantContent(t, h, target, "target")
	wantNames(t, h, sub.root, "file.before-kitbag", "moved", "target")
}

func (s suite) renameRefusesExisting(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	a, b, empty := filepath.Join(sub.root, "a"), filepath.Join(sub.root, "b"), filepath.Join(sub.root, "empty")
	mustWrite(t, h, a, "a", 0o644)
	mustWrite(t, h, b, "b", 0o644)
	mustMkdir(t, h, empty)
	mustMkdir(t, h, filepath.Join(sub.root, "dir"))
	wantErr(t, "onto file", h.Rename(a, b), fs.ErrExist)
	wantErr(t, "onto itself", h.Rename(a, a), fs.ErrExist)
	wantErr(t, "dir onto empty dir", h.Rename(filepath.Join(sub.root, "dir"), empty), fs.ErrExist)
	wantContent(t, h, a, "a")
	wantContent(t, h, b, "b")
	wantNames(t, h, sub.root, "a", "b", "dir", "empty")
}

func (s suite) renameErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, dir := sub.host, filepath.Join(sub.root, "dir")
	mustMkdir(t, h, dir)
	wantErr(t, "missing source", h.Rename(filepath.Join(sub.root, "missing"), filepath.Join(sub.root, "x")), fs.ErrNotExist)
	wantErr(t, "missing parent", h.Rename(dir, filepath.Join(sub.root, "missing", "x")), fs.ErrNotExist)
	wantErr(t, "into itself", h.Rename(dir, filepath.Join(dir, "inside")), fs.ErrInvalid)
	wantNames(t, h, sub.root, "dir")
	wantNames(t, h, dir)
}

func (s suite) renameCrossDevice(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	if sub.otherDevice == "" {
		t.Skip("no second filesystem is available to this implementation")
	}
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	mustWrite(t, h, filepath.Join(sub.otherDevice, "other"), "y", 0o644)
	wantErr(t, "out", h.Rename(file, filepath.Join(sub.otherDevice, "file")), platform.ErrCrossDevice)
	wantErr(t, "in", h.Rename(filepath.Join(sub.otherDevice, "other"), filepath.Join(sub.root, "other")), platform.ErrCrossDevice)
	wantContent(t, h, file, "x")
	wantNames(t, h, sub.otherDevice, "other")
}

func (s suite) removeEntries(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	file, dir := filepath.Join(sub.root, "file"), filepath.Join(sub.root, "dir")
	mustWrite(t, h, file, "x", 0o644)
	mustMkdir(t, h, dir)
	mustWrite(t, h, filepath.Join(dir, "keep"), "keep", 0o644)
	mustMkdir(t, h, filepath.Join(sub.root, "empty"))
	mustLink(t, h, file, filepath.Join(sub.root, "to-file"))
	mustLink(t, h, dir, filepath.Join(sub.root, "to-dir"))
	for _, name := range []string{"to-file", "to-dir", "empty"} {
		mustNil(t, "remove "+name, h.Remove(filepath.Join(sub.root, name)))
	}
	wantContent(t, h, file, "x")
	wantContent(t, h, filepath.Join(dir, "keep"), "keep")
	mustNil(t, "remove file", h.Remove(file))
	wantNames(t, h, sub.root, "dir")
}

func (s suite) removeErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, dir := sub.host, filepath.Join(sub.root, "dir")
	mustMkdir(t, h, dir)
	mustWrite(t, h, filepath.Join(dir, "keep"), "keep", 0o644)
	wantErr(t, "non-empty", h.Remove(dir), platform.ErrNotEmpty)
	wantErr(t, "missing", h.Remove(filepath.Join(sub.root, "missing")), fs.ErrNotExist)
	wantContent(t, h, filepath.Join(dir, "keep"), "keep")
}
