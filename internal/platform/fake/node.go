package fake

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Owner permission bits checked as for an unprivileged user.
const (
	permRead  fs.FileMode = 0o400
	permWrite fs.FileMode = 0o200
	permExec  fs.FileMode = 0o100
	// rootPerm is the mode of each volume's root directory.
	rootPerm fs.FileMode = 0o755
	// linkPerm is the mode reported for symbolic links, as on Linux.
	linkPerm fs.FileMode = 0o777
	// maxLinkHops bounds symbolic link resolution, like the kernel's limit.
	maxLinkHops = 40
)

var errLoop = errors.New("too many levels of symbolic links")

// node is a file, directory or symbolic link.
type node struct {
	children map[string]*node
	target   string
	data     []byte
	mode     fs.FileMode // Type and permission bits.
	dev      int
}

func newDir(perm fs.FileMode, dev int) *node {
	return &node{mode: fs.ModeDir | perm, children: map[string]*node{}, dev: dev}
}

func (n *node) isDir() bool {
	return n.mode.IsDir()
}

func (n *node) isLink() bool {
	return n.mode&fs.ModeSymlink != 0
}

func (n *node) allows(bits fs.FileMode) bool {
	return n.mode&bits == bits
}

// contains reports whether d is n or lies below it.
func (n *node) contains(d *node) bool {
	if n == d {
		return true
	}
	for _, child := range n.children {
		if child.isDir() && child.contains(d) {
			return true
		}
	}
	return false
}

// info snapshots a node so callers never share mutable state.
func info(name string, n *node) fileInfo {
	fi := fileInfo{name: name, mode: n.mode}
	switch {
	case n.isLink():
		fi.size = int64(len(n.target))
	case !n.isDir():
		fi.size = int64(len(n.data))
	}
	return fi
}

// fileInfo implements fs.FileInfo.
type fileInfo struct {
	name string
	mode fs.FileMode
	size int64
}

// Name returns the base name of the path that was described.
func (fi fileInfo) Name() string { return fi.name }

// Size returns the length of a file's data or a link's target.
func (fi fileInfo) Size() int64 { return fi.size }

// Mode returns the type and permission bits.
func (fi fileInfo) Mode() fs.FileMode { return fi.mode }

// ModTime returns the zero time; the fake keeps no clock.
func (fileInfo) ModTime() time.Time { return time.Time{} }

// IsDir reports whether the entry is a directory.
func (fi fileInfo) IsDir() bool { return fi.mode.IsDir() }

// Sys returns nil; the fake has no OS data.
func (fileInfo) Sys() any { return nil }

// dirEntry implements fs.DirEntry.
type dirEntry struct {
	info fileInfo
}

// Name returns the entry's name.
func (e dirEntry) Name() string { return e.info.name }

// IsDir reports whether the entry is a directory.
func (e dirEntry) IsDir() bool { return e.info.IsDir() }

// Type returns the entry's type bits.
func (e dirEntry) Type() fs.FileMode { return e.info.mode.Type() }

// Info returns the entry's description at listing time.
func (e dirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

// walked is a resolved path: the directory holding the final entry, the
// entry's name and the entry itself (nil when absent). parent is nil when the
// path resolved to a volume root or through a trailing "." or "..".
type walked struct {
	parent *node
	node   *node
	name   string
}

// walker resolves one path component at a time, expanding symbolic links.
type walker struct {
	h       *Host
	dirs    []*node // Directories from the volume root to the current one.
	pending []string
	hops    int
}

// walk resolves path, following a final symbolic link only when follow is set.
func (h *Host) walk(path string, follow bool) (walked, error) {
	vol := filepath.VolumeName(path)
	wk := walker{h: h, dirs: []*node{h.volume(vol)}, pending: components(path[len(vol):])}
	res := walked{node: wk.dirs[0]}
	for len(wk.pending) > 0 {
		name := wk.pending[0]
		wk.pending = wk.pending[1:]
		var err error
		if res, err = wk.step(name, follow); err != nil {
			return walked{}, err
		}
	}
	return res, nil
}

// step looks name up in the current directory, which needs search (x)
// permission as in the kernel.
func (wk *walker) step(name string, follow bool) (walked, error) {
	dir := wk.dirs[len(wk.dirs)-1]
	switch {
	case !dir.allows(permExec):
		return walked{}, fs.ErrPermission
	case name == "." || name == "..":
		return wk.dot(name), nil
	}
	child := dir.children[name]
	if len(wk.pending) > 0 {
		return wk.descend(child)
	}
	if child != nil && child.isLink() && follow {
		return wk.expand(child.target)
	}
	return walked{parent: dir, name: name, node: child}, nil
}

// descend enters an intermediate component, following a link.
func (wk *walker) descend(child *node) (walked, error) {
	switch {
	case child == nil:
		return walked{}, fs.ErrNotExist
	case child.isLink():
		return wk.expand(child.target)
	case !child.isDir():
		return walked{}, platform.ErrNotDir
	}
	wk.dirs = append(wk.dirs, child)
	return walked{node: child}, nil
}

// dot handles "." and "..", which only link targets can contain. ".." moves
// to the physical parent, as the kernel does after following links.
func (wk *walker) dot(name string) walked {
	if name == ".." && len(wk.dirs) > 1 {
		wk.dirs = wk.dirs[:len(wk.dirs)-1]
	}
	return walked{node: wk.dirs[len(wk.dirs)-1]}
}

// expand queues a link target's components ahead of the remaining ones; a
// relative target continues from the link's directory.
func (wk *walker) expand(target string) (walked, error) {
	wk.hops++
	if wk.hops > maxLinkHops {
		return walked{}, errLoop
	}
	if filepath.IsAbs(target) {
		vol := filepath.VolumeName(target)
		wk.dirs = []*node{wk.h.volume(vol)}
		target = target[len(vol):]
	}
	wk.pending = append(components(target), wk.pending...)
	return walked{node: wk.dirs[len(wk.dirs)-1]}, nil
}

// components splits a path without its volume into non-empty names.
func components(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool {
		return r == filepath.Separator || r == '/'
	})
}
