package platform_test

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// subject is one platform implementation prepared for a single test.
type subject struct {
	host platform.Host
	// opened returns the targets the host handed to its desktop opener.
	opened func() []string
	// root is an empty, writable, absolute directory owned by the test.
	root string
	// otherDevice is a writable directory on another filesystem, or "".
	otherDevice string
	// nonTerminal is a file descriptor known not to be a terminal.
	nonTerminal uintptr
	// permissions is false when permission bits are not enforced (root user).
	permissions bool
	// transientLinkReads is set where reading through a link that is being
	// replaced can fail with EINVAL (macOS).
	transientLinkReads bool
}

// factory builds a fresh subject whose environment is exactly env.
type factory func(t *testing.T, env map[string]string) subject

// suite runs the platform contract against one implementation. Every case is
// a method so that failures point at the case, and every case runs in
// parallel on its own subject.
type suite struct {
	newSubject factory
}

// runContract is the single contract that unix and fake must both satisfy.
func runContract(t *testing.T, newSubject factory) {
	t.Helper()
	s := suite{newSubject: newSubject}
	s.runArguments(t)
	s.runLinks(t)
	s.runStat(t)
	s.runWrites(t)
	s.runDirectories(t)
	s.runRenameRemove(t)
	s.runPermissions(t)
	s.runLocks(t)
	s.runEnv(t)
}

func (s suite) subject(t *testing.T) subject {
	t.Helper()
	return s.newSubject(t, nil)
}

// raw joins parts without cleaning, to build deliberately unclean paths.
func raw(parts ...string) string {
	return strings.Join(parts, string(filepath.Separator))
}

func wantErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("%s: error = %v, want %v", what, err, target)
	}
}

func mustNil(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func mustWrite(t *testing.T, h platform.FS, path, data string, perm fs.FileMode) {
	t.Helper()
	mustNil(t, "write "+path, h.WriteAtomic(path, []byte(data), perm))
}

func mustMkdir(t *testing.T, h platform.FS, path string) {
	t.Helper()
	mustNil(t, "mkdir "+path, h.MkdirAll(path, 0o755))
}

func mustLink(t *testing.T, h platform.FS, target, path string) {
	t.Helper()
	mustNil(t, "link "+path, h.Link(target, path))
}

func wantContent(t *testing.T, h platform.FS, path, want string) {
	t.Helper()
	got, err := h.ReadFile(path)
	if err != nil || !bytes.Equal(got, []byte(want)) {
		t.Errorf("ReadFile(%s) = %q, %v; want %q", path, got, err, want)
	}
}

// wantKind checks the entry type (Lstat) and, for files and directories,
// the exact permission bits.
func wantKind(t *testing.T, h platform.FS, path string, kind, perm fs.FileMode) {
	t.Helper()
	fi, err := h.Lstat(path)
	if err != nil {
		t.Errorf("Lstat(%s): %v", path, err)
		return
	}
	if fi.Mode().Type() != kind {
		t.Errorf("Lstat(%s) type = %v, want %v", path, fi.Mode().Type(), kind)
	}
	if kind != fs.ModeSymlink && fi.Mode().Perm() != perm {
		t.Errorf("Lstat(%s) perm = %v, want %v", path, fi.Mode().Perm(), perm)
	}
}

func wantNames(t *testing.T, h platform.FS, dir string, want ...string) {
	t.Helper()
	entries, err := h.ReadDir(dir)
	if err != nil {
		t.Errorf("ReadDir(%s): %v", dir, err)
		return
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, want) {
		t.Errorf("ReadDir(%s) = %q, want %q", dir, got, want)
	}
}

func wantMissing(t *testing.T, h platform.FS, path string) {
	t.Helper()
	_, err := h.Lstat(path)
	wantErr(t, "Lstat "+path, err, fs.ErrNotExist)
}

// fsCalls invokes every FS method with path in the position under test.
func fsCalls(h platform.FS, path, valid string) map[string]error {
	errOf := func(_ any, err error) error { return err }
	return map[string]error{
		"Lstat":              errOf(h.Lstat(path)),
		"Stat":               errOf(h.Stat(path)),
		"ReadFile":           errOf(h.ReadFile(path)),
		"ReadDir":            errOf(h.ReadDir(path)),
		"ReadLink":           errOf(h.ReadLink(path)),
		"IsManagedLink":      errOf(h.IsManagedLink(path, valid)),
		"IsManagedLink/root": errOf(h.IsManagedLink(valid, path)),
		"MkdirAll":           h.MkdirAll(path, 0o755),
		"WriteAtomic":        h.WriteAtomic(path, nil, 0o644),
		"Link":               h.Link(valid, path),
		"Rename/old":         h.Rename(path, valid),
		"Rename/new":         h.Rename(valid, path),
		"Remove":             h.Remove(path),
		"Chmod":              h.Chmod(path, 0o644),
		"TryLock":            errOf(h.TryLock(path)),
	}
}

func (s suite) runArguments(t *testing.T) {
	t.Helper()
	t.Run("Arguments/RejectRelativeAndUncleanPaths", s.argumentsRejectPaths)
	t.Run("Arguments/RejectNonPermissionModes", s.argumentsRejectModes)
}

func (s suite) argumentsRejectPaths(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	mustWrite(t, sub.host, filepath.Join(sub.root, "valid"), "x", 0o644)
	sep := string(filepath.Separator)
	for _, path := range []string{
		"", "relative", raw("relative", "x"), raw(sub.root, "a", "..", "b"),
		sub.root + sep, raw(sub.root, "", "a"), raw(sub.root, ".", "a"),
	} {
		for op, err := range fsCalls(sub.host, path, filepath.Join(sub.root, "valid")) {
			wantErr(t, op+"("+path+")", err, fs.ErrInvalid)
		}
	}
	wantNames(t, sub.host, sub.root, "valid")
}

func (s suite) argumentsRejectModes(t *testing.T) {
	t.Parallel()
	sub := s.subject(t)
	h, path := sub.host, filepath.Join(sub.root, "f")
	mustWrite(t, h, path, "x", 0o644)
	for _, mode := range []fs.FileMode{fs.ModeSetuid | 0o755, fs.ModeSticky | 0o777, fs.ModeDir | 0o755} {
		wantErr(t, "WriteAtomic", h.WriteAtomic(path, nil, mode), fs.ErrInvalid)
		wantErr(t, "Chmod", h.Chmod(path, mode), fs.ErrInvalid)
		wantErr(t, "MkdirAll", h.MkdirAll(filepath.Join(sub.root, "d"), mode), fs.ErrInvalid)
	}
	wantKind(t, h, path, 0, 0o644)
	wantContent(t, h, path, "x")
	wantMissing(t, h, filepath.Join(sub.root, "d"))
}
