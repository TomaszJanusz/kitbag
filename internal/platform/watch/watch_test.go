package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestDirectoryLimitsAndLinks(t *testing.T) {
	t.Parallel()
	host := fake.New()
	root := filepath.FromSlash("/repo")
	for _, path := range []string{filepath.Join(root, "sub"), filepath.Join(root, ".git", "objects"), filepath.FromSlash("/outside")} {
		mustWatch(t, host.MkdirAll(path, 0o700))
	}
	mustWatch(t, host.Link(filepath.FromSlash("/outside"), filepath.Join(root, "alias")))
	dirs, err := directories(host, []string{root, filepath.FromSlash("/missing/nested/target")})
	if err != nil || !dirs[root] || !dirs[filepath.Join(root, "sub")] || dirs[filepath.Join(root, ".git")] || dirs[filepath.FromSlash("/outside")] {
		t.Fatal(dirs, err)
	}
	deep := root
	for range depthLimit + 1 {
		deep = filepath.Join(deep, "deep")
	}
	mustWatch(t, host.MkdirAll(deep, 0o700))
	if _, err = directories(host, []string{root}); err == nil {
		t.Fatal("unbounded recursion")
	}
}

func TestRealWatcherReplacementAndClose(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	watcher, err := New()
	if err != nil {
		t.Fatal(err)
	}
	host := fake.New()
	// Real native Add receives only the explicit temporary directory; the injected
	// FS supplies its directory shape, without environment or home discovery.
	mustWatch(t, host.MkdirAll(root, 0o700))
	mustWatch(t, watcher.Rebuild(host, []string{filepath.Join(root, "target")}))
	for i := range 3 {
		temporary := filepath.Join(root, fmt.Sprintf("new-%d", i))
		mustWatch(t, os.WriteFile(temporary, []byte("changed"), 0o600))
		mustWatch(t, os.Rename(temporary, filepath.Join(root, "target")))
	}
	select {
	case <-watcher.Hints():
	case <-time.After(time.Second):
		t.Fatal("no native event")
	}
	mustWatch(t, watcher.Close())
	for range watcher.Hints() {
	} // Closed hints cannot become a daemon busy loop.
}

func mustWatch(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
