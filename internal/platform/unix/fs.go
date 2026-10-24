//go:build darwin || linux

package unix

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

// tempPattern names temporary siblings; os.CreateTemp replaces the "*".
const tempPattern = ".kitbag-*.tmp"

var errSymlinkMode = errors.New("refusing to change the mode of a symbolic link")

// Lstat describes path without following a final symbolic link.
func (*Host) Lstat(path string) (fs.FileInfo, error) {
	if err := check.Path("lstat", path); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(path)
	return fi, classify(err)
}

// Stat describes path, following symbolic links.
func (*Host) Stat(path string) (fs.FileInfo, error) {
	if err := check.Path("stat", path); err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	return fi, classify(err)
}

// ReadFile returns the contents of the file at path.
func (*Host) ReadFile(path string) ([]byte, error) {
	if err := check.Path("read", path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: reading the caller's validated absolute path is this method's purpose.
	return data, classify(err)
}

// ReadDir lists the directory at path sorted by name.
func (*Host) ReadDir(path string) ([]fs.DirEntry, error) {
	if err := check.Path("readdir", path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, classify(err)
	}
	return entries, nil
}

// ReadLink returns the target of the symbolic link at path.
func (*Host) ReadLink(path string) (string, error) {
	if err := check.Path("readlink", path); err != nil {
		return "", err
	}
	target, err := os.Readlink(path)
	if errors.Is(err, syscall.EINVAL) {
		return "", &classified{kind: platform.ErrNotLink, err: err}
	}
	return target, classify(err)
}

// IsManagedLink reports whether path is a symbolic link into one of roots.
func (*Host) IsManagedLink(path string, roots ...string) (bool, error) {
	if err := check.Path("managedlink", path); err != nil {
		return false, err
	}
	if err := check.Roots(roots); err != nil {
		return false, err
	}
	target, err := os.Readlink(path)
	switch {
	case errors.Is(err, syscall.EINVAL):
		return false, nil // An existing entry that is not a link.
	case err != nil:
		return false, classify(err)
	}
	return check.Managed(path, target, roots), nil
}

// MkdirAll creates path and its missing parents with exactly perm.
func (*Host) MkdirAll(path string, perm fs.FileMode) error {
	if err := check.Path("mkdir", path); err != nil {
		return err
	}
	if err := check.Perm("mkdir", path, perm); err != nil {
		return err
	}
	missing, err := missingDirs(path)
	if err != nil {
		return err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := mkdir(missing[i], perm); err != nil {
			return err
		}
	}
	return nil
}

// missingDirs lists path and its ancestors that do not exist, deepest first,
// and requires the nearest existing one to be a directory.
func missingDirs(path string) ([]string, error) {
	var missing []string
	for dir := path; ; dir = filepath.Dir(dir) {
		fi, err := os.Stat(dir)
		switch {
		case err == nil && fi.IsDir():
			return missing, nil
		case err == nil:
			return nil, wrap("mkdir", dir, platform.ErrNotDir)
		case !errors.Is(err, fs.ErrNotExist) || filepath.Dir(dir) == dir:
			return nil, classify(err)
		}
		missing = append(missing, dir)
	}
}

// mkdir creates one directory and then sets perm, which mkdir(2) would mask
// with the umask. A directory created concurrently is accepted as it is; a
// dangling symbolic link in the way fails with fs.ErrExist.
func mkdir(dir string, perm fs.FileMode) error {
	if err := os.Mkdir(dir, perm); err != nil {
		if fi, statErr := os.Stat(dir); statErr == nil && fi.IsDir() {
			return nil
		}
		return classify(err)
	}
	return classify(os.Chmod(dir, perm))
}

// WriteAtomic replaces the entry at path with a regular file.
func (h *Host) WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	if err := check.Path("write", path); err != nil {
		return err
	}
	if err := check.Perm("write", path, perm); err != nil {
		return err
	}
	if err := refuseDir("write", path); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return wrap("write", path, classify(err))
	}
	if err := h.fill(f, data, perm); err != nil {
		return wrap("write", path, errors.Join(err, removeTemp(f.Name())))
	}
	return wrap("write", path, h.commit(f.Name(), path))
}

// fill writes, chmods and syncs a temporary file, then closes it.
func (h *Host) fill(f *os.File, data []byte, perm fs.FileMode) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = h.syncFile(f)
	}
	return errors.Join(err, f.Close())
}

// commit renames a complete temporary entry over path and syncs the
// directory. After a failed directory sync the new entry is in place but may
// not survive a crash; the error is still reported.
func (h *Host) commit(tmp, path string) error {
	if err := h.rename(tmp, path); err != nil {
		return errors.Join(classify(err), removeTemp(tmp))
	}
	return h.syncDir(filepath.Dir(path))
}

// Link replaces the entry at path with a symbolic link to target.
func (h *Host) Link(target, path string) error {
	if err := check.Path("symlink", path); err != nil {
		return err
	}
	if err := check.LinkTarget(path, target); err != nil {
		return err
	}
	if err := refuseDir("symlink", path); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), ".kitbag-"+rand.Text()+".tmp")
	if err := os.Symlink(target, tmp); err != nil {
		return wrap("symlink", path, classify(err))
	}
	return wrap("symlink", path, h.commit(tmp, path))
}

// refuseDir fails when path is a directory, which replacement must never
// remove. A missing path is fine: creating the temporary sibling reports a
// missing parent.
func refuseDir(op, path string) error {
	fi, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return classify(err)
	case fi.IsDir():
		return wrap(op, path, platform.ErrIsDir)
	}
	return nil
}

// removeTemp deletes a temporary entry left by a failed replacement.
func removeTemp(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Rename moves oldpath to newpath, which must not exist. The existence check
// and rename are separate steps; callers serialize kitbag's own writers with
// TryLock, and no other program is expected to create kitbag's destinations.
func (h *Host) Rename(oldpath, newpath string) error {
	if err := check.Path("rename", oldpath); err != nil {
		return err
	}
	if err := check.Path("rename", newpath); err != nil {
		return err
	}
	if _, err := os.Lstat(oldpath); err != nil {
		return classify(err)
	}
	switch _, err := os.Lstat(newpath); {
	case err == nil:
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrExist}
	case !errors.Is(err, fs.ErrNotExist):
		return classify(err)
	}
	if err := h.rename(oldpath, newpath); err != nil {
		return classify(err)
	}
	oldDir, newDir := filepath.Dir(oldpath), filepath.Dir(newpath)
	if err := h.syncDir(newDir); err != nil || oldDir == newDir {
		return err
	}
	return h.syncDir(oldDir)
}

// Remove deletes a file, a symbolic link or an empty directory.
func (h *Host) Remove(path string) error {
	if err := check.Path("remove", path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return classify(err)
	}
	return h.syncDir(filepath.Dir(path))
}

// Chmod sets the permission bits of a file or directory.
func (*Host) Chmod(path string, perm fs.FileMode) error {
	if err := check.Path("chmod", path); err != nil {
		return err
	}
	if err := check.Perm("chmod", path, perm); err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return classify(err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return check.Invalid("chmod", path, errSymlinkMode)
	}
	return classify(os.Chmod(path, perm))
}

// syncDir makes a directory's entries durable. Some filesystems cannot sync a
// directory; they report EINVAL or ENOTSUP, which is not a failure of the
// write itself.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // G304: dir is the parent of a validated absolute path.
	if err != nil {
		return err
	}
	err = d.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, errors.ErrUnsupported) {
		err = nil
	}
	return errors.Join(err, d.Close())
}
