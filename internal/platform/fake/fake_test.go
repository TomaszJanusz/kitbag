package fake_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestOptions(t *testing.T) {
	t.Parallel()
	env := map[string]string{"HOME": "/home/kit"}
	h := fake.New(fake.WithEnv(env), fake.WithHostname("runner02"), fake.WithTerminal(0, 2))
	env["HOME"] = "/changed"
	if home, err := h.HomeDir(); err != nil || home != "/home/kit" {
		t.Errorf("HomeDir() = %q, %v; want the environment as passed to New", home, err)
	}
	if name, err := h.Hostname(); err != nil || name != "runner02" {
		t.Errorf("Hostname() = %q, %v; want runner02", name, err)
	}
	if !h.IsTerminal(0) || h.IsTerminal(1) || !h.IsTerminal(2) {
		t.Error("IsTerminal does not match WithTerminal(0, 2)")
	}
	if name, err := fake.New().Hostname(); err != nil || name == "" {
		t.Errorf("default Hostname() = %q, %v; want a name", name, err)
	}
}

func TestMountRequiresEmptyDirectory(t *testing.T) {
	t.Parallel()
	h := fake.New()
	if err := h.MkdirAll("/full/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic("/file", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]error{
		"/missing": fs.ErrNotExist,
		"/file":    fs.ErrInvalid,
		"/full":    fs.ErrInvalid,
		"relative": fs.ErrInvalid,
	} {
		if err := h.Mount(path); !errors.Is(err, want) {
			t.Errorf("Mount(%s) = %v, want %v", path, err, want)
		}
	}
}

func TestVolumeRootIsProtected(t *testing.T) {
	t.Parallel()
	h := fake.New()
	if fi, err := h.Lstat("/"); err != nil || !fi.IsDir() {
		t.Errorf("Lstat(/) = %v, %v; want a directory", fi, err)
	}
	if err := h.Remove("/"); err == nil {
		t.Error("Remove(/) succeeded")
	}
	if err := h.Rename("/", "/x"); err == nil {
		t.Error("Rename(/) succeeded")
	}
	if _, err := h.TryLock("/"); err == nil {
		t.Error("TryLock(/) succeeded")
	}
}

func TestRelativeLinksStopAtRoot(t *testing.T) {
	t.Parallel()
	h := fake.New()
	if err := h.WriteAtomic("/etc", []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.Link("../../../etc", "/link"); err != nil {
		t.Fatal(err)
	}
	if got, err := h.ReadFile("/link"); err != nil || string(got) != "top" {
		t.Errorf("ReadFile(/link) = %q, %v; want top", got, err)
	}
}

// TestConcurrentUse is meaningful under the race detector (mise run check).
func TestConcurrentUse(t *testing.T) {
	t.Parallel()
	h := fake.New()
	if err := h.MkdirAll("/work", 0o755); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			path := filepath.Join("/work", strconv.Itoa(i))
			for range 50 {
				if err := h.WriteAtomic(path, []byte(path), 0o644); err != nil {
					t.Error(err)
					return
				}
				if _, err := h.ReadDir("/work"); err != nil {
					t.Error(err)
					return
				}
				if err := h.Open(path); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if got := len(h.Opened()); got != 400 {
		t.Errorf("Opened() has %d targets, want 400", got)
	}
}
