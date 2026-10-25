package update

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

type observedFS struct {
	platform.FS
	t      *testing.T
	binary string
}

func (h observedFS) WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	if path == h.binary {
		if _, err := h.Lstat(path); err != nil {
			h.t.Errorf("live binary disappeared before replacement: %v", err)
		}
	}
	return h.FS.WriteAtomic(path, data, perm)
}

func TestReplacementNeverUnlinks(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.service.FS = observedFS{FS: f.host, t: t, binary: f.service.Binary}
	_, err := f.service.Run(t.Context(), Options{Channel: "stable"})
	must(t, err)
}

func TestPreexistingBackupAndLock(t *testing.T) {
	t.Parallel()
	for _, backup := range []bool{false, true} {
		t.Run(boolName(backup), func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			if backup {
				must(t, f.host.WriteAtomic(f.service.Binary+".old", []byte("preserved backup"), binaryPerm))
			} else {
				lock, err := f.host.TryLock(f.service.Binary + ".update.lock")
				must(t, err)
				defer func() { must(t, lock.Unlock()) }()
			}
			if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
				t.Fatal("accepted interrupted or concurrent update")
			}
			f.assertBinary(t, "old executable")
		})
	}
}

func boolName(b bool) string {
	if b {
		return "backup"
	}
	return "lock"
}

func TestInstallScriptAndReexecError(t *testing.T) {
	t.Parallel()
	f := setup(t)
	stamp := filepath.Join(filepath.Dir(f.service.Binary), "kitbag.install-source")
	must(t, f.host.WriteAtomic(stamp, []byte("install-script"), binaryPerm))
	f.service.Exec = func(context.Context, string, []string) error { return fs.ErrPermission }
	result, err := f.service.Run(t.Context(), Options{Channel: "stable"})
	if !errors.Is(err, fs.ErrPermission) || !result.Updated {
		t.Fatalf("result %+v, error %v", result, err)
	}
	f.assertBinary(t, "new executable")
}

func TestMissingDependencies(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"fetch", "verify", "probe", "exec", "relative path"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			switch name {
			case "fetch":
				f.service.Fetch = nil
			case "verify":
				f.service.Verify = nil
			case "probe":
				f.service.Probe = nil
			case "exec":
				f.service.Exec = nil
			case "relative path":
				f.service.Binary = "kitbag"
			}
			if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
				t.Fatal("accepted invalid service")
			}
		})
	}
}

func TestConcurrentNewerInstall(t *testing.T) {
	t.Parallel()
	f := setup(t)
	f.service.Probe = func(context.Context, string) (string, error) { return "kitbag 3.0.0", nil }
	if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
		t.Fatal("downgraded concurrently installed newer version")
	}
	f.assertBinary(t, "old executable")
}

func TestRollbackFailurePreservesBackup(t *testing.T) {
	t.Parallel()
	f := setup(t)
	must(t, f.host.InterruptWriteAfter(f.service.Binary, fake.BeforeRename, 1))
	f.service.Probe = func(context.Context, string) (string, error) {
		data, err := f.host.ReadFile(f.service.Binary)
		must(t, err)
		if string(data) == "old executable" {
			return "kitbag 1.0.0", nil
		}
		return "kitbag wrong-version", nil
	}
	if _, err := f.service.Run(t.Context(), Options{Channel: "stable"}); err == nil {
		t.Fatal("rollback reported success")
	}
	data, err := f.host.ReadFile(f.service.Binary + ".old")
	must(t, err)
	if string(data) != "old executable" {
		t.Fatal("recovery backup lost")
	}
	if f.execs != 0 {
		t.Fatal("executed after rollback failure")
	}
}
