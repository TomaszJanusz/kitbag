//go:build darwin || linux

package unix

import (
	"errors"
	"io/fs"
	"syscall"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// classified keeps the original error (and its errno) while also matching a
// platform sentinel, so messages stay native and classification portable.
type classified struct {
	kind error
	err  error
}

func (e *classified) Error() string {
	return e.err.Error()
}

func (e *classified) Unwrap() []error {
	return []error{e.kind, e.err}
}

// classify maps the errno values the platform contract names onto sentinels.
// fs.ErrNotExist, fs.ErrExist and fs.ErrPermission already match errno values.
func classify(err error) error {
	var kind error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.EXDEV):
		kind = platform.ErrCrossDevice
	case errors.Is(err, syscall.ENOTEMPTY):
		kind = platform.ErrNotEmpty
	case errors.Is(err, syscall.EISDIR):
		kind = platform.ErrIsDir
	case errors.Is(err, syscall.ENOTDIR):
		kind = platform.ErrNotDir
	case errors.Is(err, syscall.EINVAL):
		kind = fs.ErrInvalid
	default:
		return err
	}
	return &classified{kind: kind, err: err}
}

// wrap names the operation and path of a failure, keeping nil as nil.
func wrap(op, path string, err error) error {
	if err == nil {
		return nil
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}
