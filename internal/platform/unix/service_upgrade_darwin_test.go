package unix

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/doctor"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestLaunchdLibexecUpgrade(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{"bin/kitbag", "opt/kitbag/libexec/kitbag"} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()
			for _, legacy := range []bool{false, true} {
				t.Run(map[bool]string{false: "stable", true: "legacy-0.1.0"}[legacy], func(t *testing.T) {
					t.Parallel()
					checkLibexecUpgrade(t, entry, legacy)
				})
			}
		})
	}
}

func checkLibexecUpgrade(t *testing.T, entry string, legacy bool) {
	t.Helper()
	root := homebrewLayout(t, "libexec", "opt", "bin")
	home := t.TempDir()
	h := New(WithEnv(func(key string) (string, bool) {
		value, ok := map[string]string{"HOME": home, "USER": "testing"}[key]
		return value, ok
	}), WithExecutable(func() (string, error) {
		// Discovery resolves both the prefix bin chain and opt, as Executable does.
		return filepath.EvalSymlinks(filepath.Join(root, entry))
	}), WithServiceRunner(func(context.Context, string, []string, []string) (string, int, error) {
		return "state = running\nlast exit code = (never exited)", 0, nil
	}))
	s := service.Service{Host: h}
	options, err := s.Current("daemon", service.DefaultInterval)
	requireUpgrade(t, err)
	stable := filepath.Join(root, "opt", "kitbag", "libexec", "kitbag")
	if options.Binary != stable {
		t.Fatalf("binary %s, want %s", options.Binary, stable)
	}
	old := filepath.Join(root, "Cellar", "kitbag", "0.1.0", "libexec", "kitbag")
	if legacy {
		// 0.1.0 persisted the resolved Cellar path in its plist and metadata.
		options.Binary = old
	}
	_, err = s.Install(t.Context(), options, true)
	requireUpgrade(t, err)
	layout, err := h.ServiceLayout()
	requireUpgrade(t, err)
	plist := filepath.Join(layout.Directory, platform.ServiceLabel+".plist")
	before, err := h.ReadFile(plist)
	requireUpgrade(t, err)
	if !strings.Contains(string(before), "<string>"+options.Binary+"</string>") {
		t.Fatal("plist does not run the installed binary", string(before))
	}
	upgradeLibexec(t, root)
	report, err := s.Status(t.Context())
	requireUpgrade(t, err)
	if !report.Installed || report.Stale != legacy || report.BinaryMissing != legacy || report.Binary != options.Binary {
		t.Fatalf("after cleanup: %+v", report)
	}
	checkUpgradeDoctor(t, h, legacy)
	checkUpgradePlist(t, s, plist, before, root, legacy)
}

func upgradeLibexec(t *testing.T, root string) {
	t.Helper()
	keg := filepath.Join(root, "Cellar", "kitbag", "0.2.0")
	requireUpgrade(t, os.MkdirAll(filepath.Join(keg, "libexec"), 0o700))
	requireUpgrade(t, os.MkdirAll(filepath.Join(keg, "bin"), 0o700))
	requireUpgrade(t, os.WriteFile(filepath.Join(keg, "libexec", "kitbag"), nil, 0o600))
	requireUpgrade(t, os.Symlink("../libexec/kitbag", filepath.Join(keg, "bin", "kitbag")))
	for link, target := range map[string]string{
		"opt/kitbag": "../Cellar/kitbag/0.2.0",
		"bin/kitbag": "../Cellar/kitbag/0.2.0/bin/kitbag",
	} {
		path := filepath.Join(root, link)
		requireUpgrade(t, os.Remove(path))
		requireUpgrade(t, os.Symlink(target, path))
	}
	requireUpgrade(t, os.RemoveAll(filepath.Join(root, "Cellar", "kitbag", "0.1.0")))
}

func checkUpgradePlist(t *testing.T, s service.Service, plist string, before []byte, prefix string, legacy bool) {
	t.Helper()
	after, err := s.Host.ReadFile(plist)
	requireUpgrade(t, err)
	if string(before) != string(after) {
		t.Fatal("status/doctor rewrote the plist")
	}
	stable := filepath.Join(prefix, "opt", "kitbag", "libexec", "kitbag")
	current, err := s.Current("daemon", service.DefaultInterval)
	requireUpgrade(t, err)
	if current.Binary != stable {
		t.Fatalf("upgrade changed the stable path to %s", current.Binary)
	}
	if !legacy {
		resolved, resolveErr := filepath.EvalSymlinks(current.Binary)
		requireUpgrade(t, resolveErr)
		want := filepath.Join(prefix, "Cellar", "kitbag", "0.2.0", "libexec", "kitbag")
		if resolved != want {
			t.Fatalf("stable executable resolves to %s, want %s", resolved, want)
		}
		changed, installErr := s.Install(t.Context(), current, true)
		requireUpgrade(t, installErr)
		if changed {
			t.Fatal("upgrade required rewriting the plist")
		}
	}
}

func checkUpgradeDoctor(t *testing.T, h *Host, legacy bool) {
	t.Helper()
	store, err := state.New(h, h, state.Options{})
	requireUpgrade(t, err)
	report := (doctor.Service{
		Host: h, Store: store, Repository: t.TempDir(),
		Open: func(context.Context) (vcs.Backend, vcs.Resolution, error) {
			return nil, vcs.Resolution{}, fs.ErrNotExist
		},
		VersionProbe: func(context.Context, string) (string, error) { return "", fs.ErrNotExist },
	}).Run(t.Context())
	for _, check := range report.Checks {
		if check.ID != "service" {
			continue
		}
		want := doctor.OK
		if legacy {
			want = doctor.Warn
		}
		if check.Status != want || strings.Contains(check.Message, "no longer exists") != legacy || !strings.Contains(check.Message, map[bool]string{true: "stale=true", false: "stale=false"}[legacy]) {
			t.Fatalf("doctor service: %+v", check)
		}
		if legacy && !strings.Contains(check.Fix, "kitbag service refresh") {
			t.Fatal("missing repair guidance", check)
		}
		return
	}
	t.Fatal("doctor omitted the service check")
}

func requireUpgrade(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
