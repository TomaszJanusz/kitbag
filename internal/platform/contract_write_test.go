package platform_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func (s suite) runStat(t *testing.T) {
	t.Helper()
	t.Run("Stat/FollowsLinksLstatDoesNot", s.statFollowsLinks)
	t.Run("Stat/DanglingLinks", s.statDanglingLinks)
	t.Run("Stat/IntermediateLinks", s.statIntermediateLinks)
	t.Run("Stat/Errors", s.statErrors)
	t.Run("ReadDir/SortedWithTypes", s.readDirSortedWithTypes)
	t.Run("Read/Errors", s.readErrors)
}

func (s suite) statFollowsLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	file, dir := filepath.Join(sub.root, "file"), filepath.Join(sub.root, "dir")
	mustWrite(t, h, file, "hello", 0o640)
	mustMkdir(t, h, dir)
	toFile, toDir := filepath.Join(sub.root, "to-file"), filepath.Join(sub.root, "to-dir")
	mustLink(t, h, file, toFile)
	mustLink(t, h, "dir", toDir)

	wantStat(t, h.Stat, toFile, "to-file", 0)
	wantStat(t, h.Stat, toDir, "to-dir", fs.ModeDir)
	wantStat(t, h.Lstat, toFile, "to-file", fs.ModeSymlink)
	wantStat(t, h.Lstat, toDir, "to-dir", fs.ModeSymlink)
	if fi, err := h.Stat(toFile); err != nil || fi.Size() != 5 || fi.Mode().Perm() != 0o640 {
		t.Errorf("Stat(to-file) = %v, %v; want the 5-byte 0640 target", fi, err)
	}
}

// wantStat checks the name and type a stat function reports for path.
func wantStat(t *testing.T, stat func(string) (fs.FileInfo, error), path, name string, kind fs.FileMode) {
	t.Helper()
	fi, err := stat(path)
	if err != nil {
		t.Errorf("stat(%s): %v", path, err)
		return
	}
	if fi.Name() != name || fi.Mode().Type() != kind || fi.IsDir() != (kind == fs.ModeDir) {
		t.Errorf("stat(%s) = %s %v, want %s %v", path, fi.Name(), fi.Mode().Type(), name, kind)
	}
}

func (s suite) statDanglingLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, link := sub.host, filepath.Join(sub.root, "dangling")
	mustLink(t, h, filepath.Join(sub.root, "missing"), link)
	wantKind(t, h, link, fs.ModeSymlink, 0)
	_, err := h.Stat(link)
	wantErr(t, "Stat", err, fs.ErrNotExist)
	_, err = h.ReadFile(link)
	wantErr(t, "ReadFile", err, fs.ErrNotExist)
	_, err = h.ReadDir(link)
	wantErr(t, "ReadDir", err, fs.ErrNotExist)
}

func (s suite) statIntermediateLinks(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	dir := filepath.Join(sub.root, "dir")
	mustMkdir(t, h, filepath.Join(dir, "sub"))
	mustWrite(t, h, filepath.Join(dir, "sub", "file"), "deep", 0o644)
	alias := filepath.Join(sub.root, "alias")
	mustLink(t, h, dir, alias)
	chain := filepath.Join(sub.root, "chain")
	mustLink(t, h, filepath.Join("alias", "sub"), chain)

	wantContent(t, h, filepath.Join(alias, "sub", "file"), "deep")
	wantContent(t, h, filepath.Join(chain, "file"), "deep")
	wantNames(t, h, alias, "sub")
	mustWrite(t, h, filepath.Join(chain, "new"), "via link", 0o644)
	wantContent(t, h, filepath.Join(dir, "sub", "new"), "via link")
	wantKind(t, h, filepath.Join(alias, "sub"), fs.ModeDir, 0o755)
}

func (s suite) statErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	_, err := h.Stat(filepath.Join(file, "x"))
	wantErr(t, "through file", err, platform.ErrNotDir)
	_, err = h.Lstat(filepath.Join(sub.root, "missing", "x"))
	wantErr(t, "through missing", err, fs.ErrNotExist)

	one, two := filepath.Join(sub.root, "one"), filepath.Join(sub.root, "two")
	mustLink(t, h, two, one)
	mustLink(t, h, one, two)
	wantKind(t, h, one, fs.ModeSymlink, 0)
	if _, err := h.Stat(one); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(link loop) error = %v, want a loop error", err)
	}
}

func (s suite) readDirSortedWithTypes(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	mustWrite(t, h, filepath.Join(sub.root, "b"), "bb", 0o600)
	// Upper case sorts first in byte order; names differ beyond case because
	// default macOS volumes are case-insensitive.
	mustWrite(t, h, filepath.Join(sub.root, "Zeta"), "Z", 0o644)
	mustMkdir(t, h, filepath.Join(sub.root, "a"))
	mustLink(t, h, "b", filepath.Join(sub.root, "c"))
	entries, err := h.ReadDir(sub.root)
	mustNil(t, "ReadDir", err)
	want := []struct {
		name string
		kind fs.FileMode
	}{{"Zeta", 0}, {"a", fs.ModeDir}, {"b", 0}, {"c", fs.ModeSymlink}}
	if len(entries) != len(want) {
		t.Fatalf("ReadDir = %d entries, want %d", len(entries), len(want))
	}
	for i, e := range entries {
		if e.Name() != want[i].name || e.Type() != want[i].kind || e.IsDir() != (want[i].kind == fs.ModeDir) {
			t.Errorf("entry %d = %s %v, want %s %v", i, e.Name(), e.Type(), want[i].name, want[i].kind)
		}
	}
	if fi, err := entries[2].Info(); err != nil || fi.Size() != 2 || fi.Mode().Perm() != 0o600 {
		t.Errorf("Info(b) = %v, %v; want 2-byte 0600 file", fi, err)
	}
}

func (s suite) readErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	_, err := h.ReadFile(sub.root)
	wantErr(t, "ReadFile(dir)", err, platform.ErrIsDir)
	_, err = h.ReadDir(file)
	wantErr(t, "ReadDir(file)", err, platform.ErrNotDir)
	_, err = h.ReadFile(filepath.Join(sub.root, "missing"))
	wantErr(t, "ReadFile(missing)", err, fs.ErrNotExist)
	_, err = h.ReadDir(filepath.Join(sub.root, "missing"))
	wantErr(t, "ReadDir(missing)", err, fs.ErrNotExist)
}

func (s suite) runWrites(t *testing.T) {
	t.Helper()
	t.Run("WriteAtomic/ExactMode", s.writeExactMode)
	t.Run("WriteAtomic/ReplacesFiles", s.writeReplacesFiles)
	t.Run("WriteAtomic/ReplacesLinkNotTarget", s.writeReplacesLinkNotTarget)
	t.Run("WriteAtomic/RefusesDirectories", s.writeRefusesDirectories)
	t.Run("WriteAtomic/Errors", s.writeErrors)
	t.Run("WriteAtomic/LeavesNoTemporaryFiles", s.writeLeavesNoTemporaryFiles)
	t.Run("WriteAtomic/CopiesData", s.writeCopiesData)
	t.Run("WriteAtomic/AtomicForConcurrentReaders", s.writeAtomicForReaders)
	t.Run("Link/AtomicForConcurrentReaders", s.linkAtomicForReaders)
}

func (s suite) writeExactMode(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	for _, perm := range []fs.FileMode{0o400, 0o600, 0o640, 0o644, 0o755, 0o777} {
		path := filepath.Join(sub.root, perm.String())
		mustWrite(t, sub.host, path, perm.String(), perm)
		wantKind(t, sub.host, path, 0, perm)
		wantContent(t, sub.host, path, perm.String())
	}
	empty := filepath.Join(sub.root, "empty")
	mustNil(t, "write empty", sub.host.WriteAtomic(empty, nil, 0o644))
	wantContent(t, sub.host, empty, "")
}

func (s suite) writeReplacesFiles(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "f")
	mustWrite(t, h, path, "old content that is longer", 0o644)
	mustWrite(t, h, path, "new", 0o600)
	wantContent(t, h, path, "new")
	wantKind(t, h, path, 0, 0o600)

	// Replacement needs a writable directory, not a writable file.
	readOnly := filepath.Join(sub.root, "read-only")
	mustWrite(t, h, readOnly, "old", 0o444)
	mustWrite(t, h, readOnly, "new", 0o444)
	wantContent(t, h, readOnly, "new")
}

func (s suite) writeReplacesLinkNotTarget(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	repo, path := filepath.Join(sub.root, "repo-file"), filepath.Join(sub.root, "agent-file")
	mustWrite(t, h, repo, "repo", 0o644)
	mustLink(t, h, repo, path)
	mustWrite(t, h, path, "copy", 0o600)
	wantKind(t, h, path, 0, 0o600)
	wantContent(t, h, path, "copy")
	wantContent(t, h, repo, "repo")
	wantKind(t, h, repo, 0, 0o644)
}

func (s suite) writeRefusesDirectories(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, dir := sub.host, filepath.Join(sub.root, "dir")
	mustMkdir(t, h, dir)
	mustWrite(t, h, filepath.Join(dir, "keep"), "keep", 0o644)
	wantErr(t, "write over directory", h.WriteAtomic(dir, []byte("x"), 0o644), platform.ErrIsDir)
	wantErr(t, "write over root", h.WriteAtomic(sub.root, []byte("x"), 0o644), platform.ErrIsDir)
	wantNames(t, h, sub.root, "dir")
	wantContent(t, h, filepath.Join(dir, "keep"), "keep")
}

func (s suite) writeErrors(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, file := sub.host, filepath.Join(sub.root, "file")
	mustWrite(t, h, file, "x", 0o644)
	wantErr(t, "missing parent", h.WriteAtomic(filepath.Join(sub.root, "missing", "f"), nil, 0o644), fs.ErrNotExist)
	wantErr(t, "file parent", h.WriteAtomic(filepath.Join(file, "f"), nil, 0o644), platform.ErrNotDir)
	wantNames(t, h, sub.root, "file")
}

func (s suite) writeLeavesNoTemporaryFiles(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	for i := range 5 {
		mustWrite(t, h, filepath.Join(sub.root, "file"), string(rune('a'+i)), 0o644)
		mustLink(t, h, string(rune('a'+i)), filepath.Join(sub.root, "link"))
	}
	wantNames(t, h, sub.root, "file", "link")
}

func (s suite) writeCopiesData(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "f")
	data := []byte("abc")
	mustNil(t, "write", h.WriteAtomic(path, data, 0o644))
	data[0] = 'X'
	got, err := h.ReadFile(path)
	mustNil(t, "read", err)
	got[1] = 'Y'
	wantContent(t, h, path, "abc")
}

// writeAtomicForReaders alternates two large contents while reading
// concurrently: a reader must only ever see one of them complete, never an
// empty, truncated or missing file.
func (s suite) writeAtomicForReaders(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "state.json")
	contents := [][]byte{bytes.Repeat([]byte("a"), 1<<17), bytes.Repeat([]byte("b"), 1<<17)}
	mustNil(t, "seed", h.WriteAtomic(path, contents[0], 0o644))
	concurrentReads(t,
		func(i int) error { return h.WriteAtomic(path, contents[(i+1)%2], 0o644) },
		func() error { return oneOf(h, path, contents) },
	)
}

// linkAtomicForReaders repoints a link while reading: the entry must always
// be a link, and reading through it must yield a complete target. Hosts with
// transientLinkReads (EINVAL on macOS, EISDIR on Linux) may instead fail such
// a read with exactly those errors, never with a missing entry.
func (s suite) linkAtomicForReaders(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h := sub.host
	contents := [][]byte{[]byte("first"), []byte("second")}
	targets := []string{filepath.Join(sub.root, "first"), filepath.Join(sub.root, "second")}
	for i, target := range targets {
		mustNil(t, "seed", h.WriteAtomic(target, contents[i], 0o644))
	}
	path := filepath.Join(sub.root, "link")
	mustLink(t, h, targets[0], path)
	concurrentReads(t,
		func(i int) error { return h.Link(targets[(i+1)%2], path) },
		func() error {
			if fi, err := h.Lstat(path); err != nil || fi.Mode().Type() != fs.ModeSymlink {
				return fmt.Errorf("entry is not a link: %v, %w", fi, err)
			}
			err := oneOf(h, path, contents)
			if tolerableLinkRead(sub.transientLinkReads, err) {
				return nil
			}
			return err
		},
	)
}

// oneOf fails unless path currently reads as one of contents.
func oneOf(h platform.FS, path string, contents [][]byte) error {
	got, err := h.ReadFile(path)
	if err != nil {
		return err
	}
	for _, want := range contents {
		if bytes.Equal(got, want) {
			return nil
		}
	}
	return fmt.Errorf("read %d bytes matching no complete version", len(got))
}

// concurrentReads runs 100 writes in a goroutine and reads until they finish.
func concurrentReads(t *testing.T, write func(i int) error, read func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < 100 && err == nil; i++ {
			err = write(i)
		}
		done <- err
	}()
	for reads := 1; ; reads++ {
		select {
		case err := <-done:
			mustNil(t, "concurrent write", err)
			return
		default:
		}
		if err := read(); err != nil {
			t.Errorf("read %d during writes: %v", reads, err)
			mustNil(t, "concurrent write", <-done)
			return
		}
	}
}
