package daemonhost

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const executeBits fs.FileMode = 0o111

// Executable detects that the daemon's own executable file was replaced, for
// example by a package upgrade. Path is followed through links, so a package
// manager's stable link that moves to a new version counts as a replacement.
//
// Replaced never restarts into a missing, non-executable, half-written or
// identical file: a changed identity (size, modification time, mode) must be
// observed unchanged by two consecutive checks, and its content digest must
// differ from the file the daemon started from. Identical content becomes the
// new baseline, so a reinstall of the same version never restarts. A restarted
// daemon records its own baseline, so one replacement restarts it once.
type Executable struct {
	fsys            platform.FS
	digest          func(string) ([]byte, error)
	path            string
	sum             []byte
	base, candidate identity
	pending         bool
}

type identity struct {
	size, modTime int64
	mode          fs.FileMode
}

// WatchExecutable records the identity and digest of path now. Digest streams
// a file's content hash; it is called again only for a settled candidate.
func WatchExecutable(fsys platform.FS, path string, digest func(string) ([]byte, error)) (*Executable, error) {
	info, err := executableInfo(fsys, path)
	if err != nil {
		return nil, err
	}
	sum, err := digest(path)
	if err != nil {
		return nil, err
	}
	return &Executable{fsys: fsys, digest: digest, path: path, sum: sum, base: identityOf(info)}, nil
}

// Path is the watched executable path.
func (e *Executable) Path() string { return e.path }

// Replaced reports a settled replacement with different content. Errors mean
// the file is currently missing or unusable; the daemon keeps running.
func (e *Executable) Replaced(context.Context) (bool, error) {
	info, err := executableInfo(e.fsys, e.path)
	if err != nil {
		e.pending = false
		return false, err
	}
	current := identityOf(info)
	if current == e.base {
		e.pending = false
		return false, nil
	}
	if !e.pending || current != e.candidate {
		e.candidate, e.pending = current, true
		return false, nil
	}
	e.pending = false
	sum, err := e.digest(e.path)
	if err != nil {
		return false, err
	}
	if bytes.Equal(sum, e.sum) {
		e.base = current
		return false, nil
	}
	return true, nil
}

func executableInfo(fsys platform.FS, path string) (fs.FileInfo, error) {
	info, err := fsys.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&executeBits == 0 {
		return nil, fmt.Errorf("%s is not an executable file: %w", path, fs.ErrInvalid)
	}
	return info, nil
}

func identityOf(info fs.FileInfo) identity {
	return identity{size: info.Size(), modTime: info.ModTime().UnixNano(), mode: info.Mode()}
}
