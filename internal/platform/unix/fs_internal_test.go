//go:build darwin || linux

package unix

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

var errInjected = errors.New("injected failure")

func noEnv(string) (string, bool) { return "", false }

// recorder wraps the durability hooks, logging each call in order.
type recorder struct {
	events []string
}

func (r *recorder) install(h *Host) {
	h.syncFile = func(f *os.File) error {
		r.events = append(r.events, "sync file")
		return f.Sync()
	}
	h.rename = func(oldpath, newpath string) error {
		r.events = append(r.events, "rename")
		return os.Rename(oldpath, newpath)
	}
	h.syncDir = func(dir string) error {
		r.events = append(r.events, "sync dir "+filepath.Base(dir))
		return syncDir(dir)
	}
}

func (r *recorder) want(t *testing.T, what string, want ...string) {
	t.Helper()
	if !slices.Equal(r.events, want) {
		t.Errorf("%s events = %q, want %q", what, r.events, want)
	}
	r.events = nil
}

func TestDurabilityOrder(t *testing.T) {
	t.Parallel()
	h, rec := New(WithEnv(noEnv)), &recorder{}
	rec.install(h)
	root := t.TempDir()
	dir := "sync dir " + filepath.Base(root)
	path := filepath.Join(root, "state.json")

	if err := h.WriteAtomic(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec.want(t, "WriteAtomic", "sync file", "rename", dir)
	if err := h.Link(path, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	rec.want(t, "Link", "rename", dir)
	if err := h.Rename(path, path+".bak"); err != nil {
		t.Fatal(err)
	}
	rec.want(t, "Rename", "rename", dir)
	if err := h.Remove(path + ".bak"); err != nil {
		t.Fatal(err)
	}
	rec.want(t, "Remove", dir)
}

func TestFailedWriteKeepsOldContent(t *testing.T) {
	t.Parallel()
	for name, inject := range map[string]func(h *Host){
		"sync file": func(h *Host) { h.syncFile = func(*os.File) error { return errInjected } },
		"rename":    func(h *Host) { h.rename = func(string, string) error { return errInjected } },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := New(WithEnv(noEnv))
			root := t.TempDir()
			path := filepath.Join(root, "f")
			if err := h.WriteAtomic(path, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}
			inject(h)
			if err := h.WriteAtomic(path, []byte("new"), 0o644); !errors.Is(err, errInjected) {
				t.Errorf("WriteAtomic error = %v, want the injected failure", err)
			}
			if name == "rename" {
				if err := h.Link(path, filepath.Join(root, "l")); !errors.Is(err, errInjected) {
					t.Errorf("Link error = %v, want the injected failure", err)
				}
			}
			wantOnly(t, root, "f", "old")
		})
	}
}

// wantOnly checks that dir holds exactly name with content: no temporary
// sibling survived a failure.
func wantOnly(t *testing.T, dir, name, content string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != name {
		t.Errorf("ReadDir = %v, %v; want only %s", entries, err, name)
	}
	if got, err := New(WithEnv(noEnv)).ReadFile(filepath.Join(dir, name)); err != nil || string(got) != content {
		t.Errorf("content = %q, %v; want %q", got, err, content)
	}
}

// TestFailedDirectorySyncIsReported shows the documented edge: the new file
// is in place, but the caller learns that durability is not confirmed.
func TestFailedDirectorySyncIsReported(t *testing.T) {
	t.Parallel()
	h := New(WithEnv(noEnv))
	h.syncDir = func(string) error { return errInjected }
	root := t.TempDir()
	if err := h.WriteAtomic(filepath.Join(root, "f"), []byte("new"), 0o644); !errors.Is(err, errInjected) {
		t.Errorf("WriteAtomic error = %v, want the injected failure", err)
	}
	wantOnly(t, root, "f", "new")
}

func TestClassify(t *testing.T) {
	t.Parallel()
	if classify(nil) != nil {
		t.Error("classify(nil) != nil")
	}
	for _, tc := range []struct {
		want  error
		errno syscall.Errno
	}{
		{want: platform.ErrCrossDevice, errno: syscall.EXDEV},
		{want: platform.ErrNotEmpty, errno: syscall.ENOTEMPTY},
		{want: platform.ErrIsDir, errno: syscall.EISDIR},
		{want: platform.ErrNotDir, errno: syscall.ENOTDIR},
		{want: fs.ErrInvalid, errno: syscall.EINVAL},
		{want: fs.ErrNotExist, errno: syscall.ENOENT},
		{want: fs.ErrPermission, errno: syscall.EACCES},
	} {
		orig := &fs.PathError{Op: "op", Path: "/p", Err: tc.errno}
		err := classify(orig)
		if !errors.Is(err, tc.want) || !errors.Is(err, tc.errno) || err.Error() != orig.Error() {
			t.Errorf("classify(%v) = %v, want a native message matching %v and the errno", tc.errno, err, tc.want)
		}
	}
}
