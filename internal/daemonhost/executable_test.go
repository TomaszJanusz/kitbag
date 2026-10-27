package daemonhost

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

const executablePerm fs.FileMode = 0o755

type watchedExecutable struct {
	host    *fake.Host
	watch   *Executable
	path    string
	digests int
}

func newWatchedExecutable(t *testing.T) *watchedExecutable {
	t.Helper()
	w := &watchedExecutable{host: fake.New(), path: filepath.FromSlash("/opt/kitbag/bin/kitbag")}
	mustLog(t, w.host.MkdirAll(filepath.Dir(w.path), privateDir))
	w.replace(t, "version 1", executablePerm)
	watch, err := WatchExecutable(w.host, w.path, w.digest)
	if err != nil {
		t.Fatal(err)
	}
	w.watch = watch
	return w
}

func (w *watchedExecutable) digest(path string) ([]byte, error) {
	w.digests++
	data, err := w.host.ReadFile(path)
	sum := sha256.Sum256(data)
	return sum[:], err
}

func (w *watchedExecutable) replace(t *testing.T, content string, perm fs.FileMode) {
	t.Helper()
	mustLog(t, w.host.WriteAtomic(w.path, []byte(content), perm))
}

// expect checks a sequence of error-free Replaced answers.
func (w *watchedExecutable) expect(t *testing.T, want ...bool) {
	t.Helper()
	for i, replaced := range want {
		got, err := w.watch.Replaced(t.Context())
		if err != nil || got != replaced {
			t.Fatalf("check %d: replaced=%t err=%v, want %t", i, got, err, replaced)
		}
	}
}

func TestExecutableReplacementSettlesBeforeRestart(t *testing.T) {
	t.Parallel()
	w := newWatchedExecutable(t)
	w.expect(t, false, false)
	w.replace(t, "version 2, longer", executablePerm)
	// The first observation only records a candidate; a stable second confirms.
	w.expect(t, false, true)
	if w.digests != 2 {
		t.Fatalf("digests=%d, want baseline plus one confirmation", w.digests)
	}
}

func TestExecutableStillBeingWrittenIsNotConfirmed(t *testing.T) {
	t.Parallel()
	w := newWatchedExecutable(t)
	w.replace(t, "partial", executablePerm)
	w.expect(t, false)
	w.replace(t, "partial write, now complete", executablePerm)
	w.expect(t, false, true)
}

func TestIdenticalExecutableBecomesBaselineWithoutRestart(t *testing.T) {
	t.Parallel()
	w := newWatchedExecutable(t)
	// Reinstalling the same bytes with another identity (here its mode) never
	// restarts, and the adopted baseline is not hashed again.
	w.replace(t, "version 1", 0o700)
	w.expect(t, false, false, false, false)
	if w.digests != 2 {
		t.Fatalf("digests=%d, want baseline plus one comparison", w.digests)
	}
	w.replace(t, "version 2, longer", 0o700)
	w.expect(t, false, true)
}

func TestMissingOrUnusableExecutableNeverRestarts(t *testing.T) {
	t.Parallel()
	w := newWatchedExecutable(t)
	w.replace(t, "version 2, longer", executablePerm)
	w.expect(t, false)
	mustLog(t, w.host.Remove(w.path))
	for range 3 {
		if replaced, err := w.watch.Replaced(t.Context()); replaced || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("missing executable: replaced=%t err=%v", replaced, err)
		}
	}
	w.replace(t, "version 2, longer", 0o600)
	for range 3 {
		if replaced, err := w.watch.Replaced(t.Context()); replaced || !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("non-executable file: replaced=%t err=%v", replaced, err)
		}
	}
	// The earlier candidate does not count: a reappearing file settles again.
	w.replace(t, "version 2, longer", executablePerm)
	w.expect(t, false, true)
}

func TestWatchExecutableRequiresAnExecutableFile(t *testing.T) {
	t.Parallel()
	host := fake.New()
	path := filepath.FromSlash("/kitbag")
	digest := func(string) ([]byte, error) { return nil, nil }
	if _, err := WatchExecutable(host, path, digest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	mustLog(t, host.WriteAtomic(path, []byte("x"), 0o600))
	if _, err := WatchExecutable(host, path, digest); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	mustLog(t, host.WriteAtomic(path, []byte("x"), executablePerm))
	failing := func(string) ([]byte, error) { return nil, fs.ErrPermission }
	if _, err := WatchExecutable(host, path, failing); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}

// Model Homebrew's stable opt link rotating before cleanup deletes the old keg.
// Stat and the digest must follow the link rather than hash its link text.
func TestExecutableFollowsStableLinkAcrossCellarCleanup(t *testing.T) {
	t.Parallel()
	w := newWatchedExecutable(t)
	old := filepath.FromSlash("/Cellar/kitbag/0.1.2/bin/kitbag")
	next := filepath.FromSlash("/Cellar/kitbag/0.1.3/bin/kitbag")
	mustLog(t, w.host.MkdirAll(filepath.Dir(old), privateDir))
	mustLog(t, w.host.MkdirAll(filepath.Dir(next), privateDir))
	mustLog(t, w.host.WriteAtomic(old, []byte("version 1"), executablePerm))
	mustLog(t, w.host.Remove(w.path))
	mustLog(t, w.host.Link(old, w.path))
	watch, err := WatchExecutable(w.host, w.path, w.digest)
	if err != nil {
		t.Fatal(err)
	}
	w.watch = watch
	// An identical-content keg with a different mode is adopted without restart.
	mustLog(t, w.host.WriteAtomic(next, []byte("version 1"), 0o700))
	mustLog(t, w.host.Remove(w.path))
	mustLog(t, w.host.Link(next, w.path))
	mustLog(t, w.host.Remove(old))
	w.expect(t, false, false, false)
	mustLog(t, w.host.WriteAtomic(next, []byte("version 2 is longer"), 0o700))
	w.expect(t, false, true)
}
