// Package check validates platform arguments identically for every
// implementation, so unix and fake reject the same inputs before touching a
// filesystem.
package check

import (
	"errors"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

var (
	errNotAbsolute = errors.New("path is not absolute")
	errNotClean    = errors.New("path is not clean")
	errBadPerm     = errors.New("mode has bits other than permissions")
	errBadTarget   = errors.New("target is neither an absolute path nor an http(s) URL")
	errEmptyTarget = errors.New("link target is empty")
	errBadUnit     = errors.New("unit name is not a single plain name")
)

// invalid wraps a reason so that it matches fs.ErrInvalid.
type invalid struct {
	reason error
}

func (e *invalid) Error() string {
	return e.reason.Error()
}

func (e *invalid) Unwrap() []error {
	return []error{fs.ErrInvalid, e.reason}
}

// Path requires an absolute, clean path.
func Path(op, path string) error {
	var reason error
	switch {
	case !filepath.IsAbs(path):
		reason = errNotAbsolute
	case filepath.Clean(path) != path:
		reason = errNotClean
	default:
		return nil
	}
	return &fs.PathError{Op: op, Path: path, Err: &invalid{reason: reason}}
}

// Perm requires a mode made of permission bits only.
func Perm(op, path string, perm fs.FileMode) error {
	if perm&^fs.ModePerm != 0 {
		return &fs.PathError{Op: op, Path: path, Err: &invalid{reason: errBadPerm}}
	}
	return nil
}

// LinkTarget requires a non-empty symbolic link target.
func LinkTarget(path, target string) error {
	if target == "" {
		return &fs.PathError{Op: "symlink", Path: path, Err: &invalid{reason: errEmptyTarget}}
	}
	return nil
}

// OpenTarget accepts an absolute clean path or an http(s) URL with a host.
// Restricting the schemes keeps manifest content from launching arbitrary
// protocol handlers, and no accepted target can start with "-".
func OpenTarget(target string) error {
	if Path("open", target) == nil {
		return nil
	}
	u, err := url.Parse(target)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return nil
	}
	return &fs.PathError{Op: "open", Path: target, Err: &invalid{reason: errBadTarget}}
}

// Invalid returns an error for op on path that matches fs.ErrInvalid.
func Invalid(op, path string, reason error) error {
	return &fs.PathError{Op: op, Path: path, Err: &invalid{reason: reason}}
}

// ResolveLink returns the lexical destination of a link at path with target:
// an absolute target cleaned, or a relative one joined to the link's directory.
func ResolveLink(path, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(filepath.Dir(path), target)
}

// Within reports whether the clean absolute path equals root or lies below it,
// comparing whole components.
func Within(root, path string) bool {
	if path == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

// Managed reports whether a link at path with target resolves within a root.
func Managed(path, target string, roots []string) bool {
	dest := ResolveLink(path, target)
	for _, root := range roots {
		if Within(root, dest) {
			return true
		}
	}
	return false
}

// Roots validates IsManagedLink roots.
func Roots(roots []string) error {
	for _, root := range roots {
		if err := Path("managedlink", root); err != nil {
			return err
		}
	}
	return nil
}

// Unit accepts a user unit name (a systemd unit file name or a launchd label):
// one non-empty path element of printable characters other than spaces, not
// starting with "-", so it can name neither another domain nor an option.
func Unit(name string) error {
	bad := name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") ||
		strings.ContainsFunc(name, func(r rune) bool { return r <= ' ' || r == 0x7f || r == '/' || r == '\\' })
	if bad {
		return Invalid("unit", name, errBadUnit)
	}
	return nil
}
