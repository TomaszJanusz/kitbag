//go:build darwin || linux

package platform_test

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

// TestUnixContract runs against real temporary directories and a fixed
// environment; it never reads the real HOME and never launches an opener.
func TestUnixContract(t *testing.T) {
	t.Parallel()
	runContract(t, newUnixSubject)
}

func newUnixSubject(t *testing.T, env map[string]string) subject {
	t.Helper()
	var (
		mu     sync.Mutex
		opened []string
	)
	h := unix.New(
		unix.WithEnv(func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		}),
		unix.WithCommandRunner(func(_ string, args ...string) error {
			mu.Lock()
			defer mu.Unlock()
			opened = append(opened, args...)
			return nil
		}),
	)
	r, w, err := os.Pipe()
	mustNil(t, "pipe", err)
	t.Cleanup(func() {
		if err := errors.Join(r.Close(), w.Close()); err != nil {
			t.Errorf("close pipe: %v", err)
		}
	})
	root := t.TempDir()
	return subject{
		host:        h,
		root:        root,
		socketRoot:  shortSocketRoot,
		permissions: os.Geteuid() != 0,
		// Reads through a link that a concurrent rename replaces can fail
		// transiently; the entry itself is never missing.
		transientLinkReads: transientLinkReadErrors(runtime.GOOS),
		otherDevice:        otherDevice(t, root),
		nonTerminal:        r.Fd(),
		opened: func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), opened...)
		},
	}
}

// otherDevice returns a temporary directory on a filesystem other than
// root's, or "" when none is available (macOS runners have no /dev/shm).
func otherDevice(t *testing.T, root string) string {
	t.Helper()
	const shm = "/dev/shm"
	if same, known := sameDevice(shm, root); same || !known {
		return ""
	}
	dir, err := os.MkdirTemp(shm, "kitbag-xdev-")
	if err != nil {
		return ""
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove %s: %v", dir, err)
		}
	})
	return dir
}

// sameDevice reports whether a and b are on one filesystem, and whether that
// could be determined.
func sameDevice(a, b string) (same, known bool) {
	sa, okA := statT(a)
	sb, okB := statT(b)
	if !okA || !okB {
		return false, false
	}
	return sa.Dev == sb.Dev, true
}

func statT(path string) (*syscall.Stat_t, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return st, ok
}

func shortSocketRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "kitbag-sock-")
	mustNil(t, "short socket directory", err)
	t.Cleanup(func() { mustNil(t, "remove socket directory", os.RemoveAll(root)) })
	return root
}
