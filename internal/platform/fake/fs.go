package fake

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

// lockPerm is the mode of a lock file created by TryLock.
const lockPerm fs.FileMode = 0o600

var (
	errSymlinkMode = errors.New("refusing to change the mode of a symbolic link")
	errSymlinkLock = errors.New("refusing to lock through a symbolic link")
	errRoot        = errors.New("refusing to change a volume root")
)

// Lstat describes path without following a final symbolic link.
func (h *Host) Lstat(path string) (fs.FileInfo, error) {
	return h.stat("lstat", path, false)
}

// Stat describes path, following symbolic links.
func (h *Host) Stat(path string) (fs.FileInfo, error) {
	return h.stat("stat", path, true)
}

func (h *Host) stat(op, path string, follow bool) (fs.FileInfo, error) {
	if err := check.Path(op, path); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing(op, path, follow)
	if err != nil {
		return nil, err
	}
	return info(filepath.Base(path), n), nil
}

// ReadFile returns a copy of the file at path.
func (h *Host) ReadFile(path string) ([]byte, error) {
	if err := check.Path("read", path); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("read", path, true)
	switch {
	case err != nil:
		return nil, err
	case !n.allows(permRead):
		return nil, wrap("read", path, fs.ErrPermission)
	case n.isDir():
		return nil, wrap("read", path, platform.ErrIsDir)
	}
	return bytes.Clone(n.data), nil
}

// ReadDir lists the directory at path sorted by name.
func (h *Host) ReadDir(path string) ([]fs.DirEntry, error) {
	if err := check.Path("readdir", path); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("readdir", path, true)
	switch {
	case err != nil:
		return nil, err
	case !n.allows(permRead):
		return nil, wrap("readdir", path, fs.ErrPermission)
	case !n.isDir():
		return nil, wrap("readdir", path, platform.ErrNotDir)
	}
	entries := make([]fs.DirEntry, 0, len(n.children))
	for name, child := range n.children {
		entries = append(entries, dirEntry{info: info(name, child)})
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

// ReadLink returns the target of the symbolic link at path.
func (h *Host) ReadLink(path string) (string, error) {
	if err := check.Path("readlink", path); err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("readlink", path, false)
	if err != nil {
		return "", err
	}
	if !n.isLink() {
		return "", wrap("readlink", path, platform.ErrNotLink)
	}
	return n.target, nil
}

// IsManagedLink reports whether path is a symbolic link into one of roots.
func (h *Host) IsManagedLink(path string, roots ...string) (bool, error) {
	if err := check.Path("managedlink", path); err != nil {
		return false, err
	}
	if err := check.Roots(roots); err != nil {
		return false, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("managedlink", path, false)
	if err != nil {
		return false, err
	}
	return n.isLink() && check.Managed(path, n.target, roots), nil
}

// MkdirAll creates path and its missing parents with exactly perm.
func (h *Host) MkdirAll(path string, perm fs.FileMode) error {
	if err := check.Path("mkdir", path); err != nil {
		return err
	}
	if err := check.Perm("mkdir", path, perm); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	vol := filepath.VolumeName(path)
	dir := vol + string(filepath.Separator)
	for _, name := range components(path[len(vol):]) {
		dir = filepath.Join(dir, name)
		if err := h.mkdir(dir, perm); err != nil {
			return wrap("mkdir", dir, err)
		}
	}
	return nil
}

// mkdir ensures one directory exists, accepting a link to a directory. As
// with mkdir(2), a dangling symbolic link in the way fails with fs.ErrExist.
func (h *Host) mkdir(dir string, perm fs.FileMode) error {
	w, err := h.walk(dir, false)
	switch {
	case err != nil:
		return err
	case w.node == nil && !w.parent.allows(permWrite):
		return fs.ErrPermission
	case w.node == nil:
		w.parent.children[w.name] = newDir(perm, w.parent.dev)
		return nil
	}
	n, err := h.mkdirTarget(dir, w.node)
	if err == nil && !n.isDir() {
		err = platform.ErrNotDir
	}
	return err
}

// mkdirTarget resolves an existing entry in mkdir's way: a link counts as
// the entry it leads to, and a link leading nowhere is fs.ErrExist.
func (h *Host) mkdirTarget(dir string, n *node) (*node, error) {
	if !n.isLink() {
		return n, nil
	}
	t, err := h.walk(dir, true)
	switch {
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return nil, err
	case err != nil || t.node == nil:
		return nil, fs.ErrExist
	}
	return t.node, nil
}

// WriteAtomic replaces the entry at path with a regular file.
func (h *Host) WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	if err := check.Path("write", path); err != nil {
		return err
	}
	if err := check.Perm("write", path, perm); err != nil {
		return err
	}
	return h.replace("write", path, &node{mode: perm, data: bytes.Clone(data)})
}

// Link replaces the entry at path with a symbolic link to target.
func (h *Host) Link(target, path string) error {
	if err := check.Path("symlink", path); err != nil {
		return err
	}
	if err := check.LinkTarget(path, target); err != nil {
		return err
	}
	return h.replace("symlink", path, &node{mode: fs.ModeSymlink | linkPerm, target: target})
}

// replace installs n at path unless a directory is there, in one step.
func (h *Host) replace(op, path string, n *node) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	w, err := h.walk(path, false)
	switch {
	case err != nil:
		return wrap(op, path, err)
	case w.parent == nil || (w.node != nil && w.node.isDir()):
		return wrap(op, path, platform.ErrIsDir)
	case !w.parent.allows(permWrite):
		return wrap(op, path, fs.ErrPermission)
	}
	n.dev = w.parent.dev
	if op == "write" {
		if err := h.interruptWrite(path, n); err != nil {
			return wrap(op, path, err)
		}
	}
	w.parent.children[w.name] = n
	return nil
}

// Rename moves oldpath to newpath, which must not exist.
func (h *Host) Rename(oldpath, newpath string) error {
	if err := check.Path("rename", oldpath); err != nil {
		return err
	}
	if err := check.Path("rename", newpath); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	from, err := h.walk(oldpath, false)
	if err == nil && from.node == nil {
		err = fs.ErrNotExist
	}
	if err != nil {
		return wrap("rename", oldpath, err)
	}
	to, err := h.walk(newpath, false)
	if err == nil && to.node != nil {
		err = fs.ErrExist
	}
	if err == nil {
		err = renameAllowed(from, to)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	delete(from.parent.children, from.name)
	to.parent.children[to.name] = from.node
	return nil
}

// renameAllowed applies rename(2)'s checks once both ends are resolved.
func renameAllowed(from, to walked) error {
	switch {
	case from.parent == nil || to.parent == nil:
		return errRoot
	case !from.parent.allows(permWrite) || !to.parent.allows(permWrite):
		return fs.ErrPermission
	case from.node.isDir() && from.node.contains(to.parent):
		return fs.ErrInvalid
	case from.node.dev != to.parent.dev:
		return platform.ErrCrossDevice
	}
	return nil
}

// Remove deletes a file, a symbolic link or an empty directory.
func (h *Host) Remove(path string) error {
	if err := check.Path("remove", path); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	w, err := h.walk(path, false)
	switch {
	case err != nil:
		return wrap("remove", path, err)
	case w.node == nil:
		return wrap("remove", path, fs.ErrNotExist)
	case w.parent == nil:
		return wrap("remove", path, errRoot)
	case !w.parent.allows(permWrite):
		return wrap("remove", path, fs.ErrPermission)
	case len(w.node.children) > 0:
		return wrap("remove", path, platform.ErrNotEmpty)
	}
	delete(w.parent.children, w.name)
	return nil
}

// Chmod sets the permission bits of a file or directory.
func (h *Host) Chmod(path string, perm fs.FileMode) error {
	if err := check.Path("chmod", path); err != nil {
		return err
	}
	if err := check.Perm("chmod", path, perm); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("chmod", path, false)
	if err != nil {
		return err
	}
	if n.isLink() {
		return check.Invalid("chmod", path, errSymlinkMode)
	}
	n.mode = n.mode.Type() | perm
	return nil
}

// TryLock takes an exclusive lock on the file at path without waiting.
func (h *Host) TryLock(path string) (platform.Unlocker, error) {
	if err := check.Path("lock", path); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	w, err := h.walk(path, false)
	if err != nil {
		return nil, wrap("lock", path, err)
	}
	n, err := lockTarget(w, path)
	if err != nil {
		return nil, err
	}
	if h.locks[n] {
		return nil, wrap("lock", path, platform.ErrLocked)
	}
	h.locks[n] = true
	return &lock{h: h, n: n}, nil
}

// lockTarget returns the file to lock, creating it as open(2) with O_CREAT
// would.
func lockTarget(w walked, path string) (*node, error) {
	n := w.node
	switch {
	case n == nil && w.parent == nil:
		return nil, wrap("lock", path, platform.ErrIsDir)
	case n == nil && !w.parent.allows(permWrite):
		return nil, wrap("lock", path, fs.ErrPermission)
	case n == nil:
		n = &node{mode: lockPerm, dev: w.parent.dev}
		w.parent.children[w.name] = n
	case n.isDir():
		return nil, wrap("lock", path, platform.ErrIsDir)
	case n.isLink():
		return nil, check.Invalid("lock", path, errSymlinkLock)
	case !n.allows(permRead):
		return nil, wrap("lock", path, fs.ErrPermission)
	}
	return n, nil
}

// lock is a held TryLock lock, identified by its file node like flock(2)
// identifies the open file.
type lock struct {
	h *Host
	n *node
}

// Unlock releases the lock.
func (l *lock) Unlock() error {
	l.h.mu.Lock()
	defer l.h.mu.Unlock()
	if l.n == nil {
		return fs.ErrClosed
	}
	delete(l.h.locks, l.n)
	l.n = nil
	return nil
}
