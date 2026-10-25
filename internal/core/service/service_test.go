package service

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func subject(t *testing.T) (Service, *fake.Host, Options) {
	t.Helper()
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "PATH": "/usr/bin:/bin", "OPENAI_API_KEY": "secret-do-not-copy"}))
	supervisor := Service{Host: host}
	options, err := supervisor.Current("daemon", DefaultInterval)
	must(t, err)
	return supervisor, host, options
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLifecycle(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"daemon", "timer"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			options.Mode = mode
			before, err := s.Status(t.Context())
			must(t, err)
			if before.Installed {
				t.Fatal(before)
			}
			changed, err := s.Install(t.Context(), options, false)
			must(t, err)
			if !changed {
				t.Fatal("first installation did not change anything")
			}
			installed, err := s.Status(t.Context())
			must(t, err)
			if !installed.Installed || !installed.Active || installed.Stale || installed.Mode != mode {
				t.Fatal(installed)
			}
			checkIdempotent(t, s, host, options)
			must(t, s.Uninstall(t.Context()))
			must(t, s.Uninstall(t.Context()))
			after, err := s.Status(t.Context())
			must(t, err)
			if after.Installed {
				t.Fatal("unit left after uninstall", after)
			}
			layout, err := host.ServiceLayout()
			must(t, err)
			entries, err := host.ReadDir(layout.Directory)
			must(t, err)
			if len(entries) != 0 {
				t.Fatal(entries)
			}
		})
	}
}

func TestStaleAndModeSwitch(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, true)
	must(t, err)
	report, err := s.Status(t.Context())
	must(t, err)
	if report.Active || !report.Enabled {
		t.Fatal(report)
	}
	layout, err := host.ServiceLayout()
	must(t, err)
	path := filepath.Join(layout.Directory, platform.ServiceLabel+".service")
	must(t, host.WriteAtomic(path, []byte("stale"), filePerm))
	report, err = s.Status(t.Context())
	must(t, err)
	if !report.Stale {
		t.Fatal("stale unit not detected")
	}
	options.Mode = "timer"
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	options.Mode = "daemon"
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	_, err = host.Lstat(filepath.Join(layout.Directory, platform.ServiceLabel+".timer"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("obsolete timer retained", err)
	}
	data, err := host.ReadFile(path)
	must(t, err)
	if strings.Contains(string(data), "secret-do-not-copy") {
		t.Fatal("secret leaked")
	}
	info, err := host.Lstat(path)
	must(t, err)
	if info.Mode().Perm() != filePerm {
		t.Fatal(info.Mode())
	}
}

func TestMissingMetadataRepair(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	metadata, err := s.metadata()
	must(t, err)
	must(t, host.Remove(metadata))
	report, err := s.Status(t.Context())
	must(t, err)
	if !report.Stale {
		t.Fatal(report)
	}
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	report, err = s.Status(t.Context())
	must(t, err)
	if report.Stale {
		t.Fatal("install did not repair metadata")
	}
}

func TestInvalidInputs(t *testing.T) {
	t.Parallel()
	_, _, valid := subject(t)
	for _, mutate := range []func(*Options){
		func(o *Options) { o.Binary = "relative" }, func(o *Options) { o.Config = "/bad\nconfig" },
		func(o *Options) { o.Mode = "invalid" }, func(o *Options) { o.Interval = 0 },
		func(o *Options) { o.Interval = time.Nanosecond }, func(o *Options) { o.Interval = 25 * time.Hour },
		func(o *Options) { o.Environment = map[string]string{"TOKEN": "secret"} }, func(o *Options) { o.Environment = map[string]string{"PATH": "/bad\npath"} },
	} {
		options := valid
		mutate(&options)
		if _, err := Generate("systemd", options); !errors.Is(err, fs.ErrInvalid) {
			t.Fatal("accepted invalid options", err)
		}
	}
	if _, err := Generate("windows", valid); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}

func checkIdempotent(t *testing.T, s Service, host *fake.Host, options Options) {
	t.Helper()
	calls := len(host.ServiceCalls())
	changed, err := s.Install(t.Context(), options, false)
	must(t, err)
	if changed {
		t.Fatal("identical install changed unit")
	}
	for _, call := range host.ServiceCalls()[calls:] {
		if strings.HasPrefix(call, "stop") || strings.HasPrefix(call, "start") {
			t.Fatal("idempotent install restarted job", call)
		}
	}
}

type launchHost struct{ *fake.Host }

// ServiceLayout exercises launchd policy with an in-memory host.
func (h launchHost) ServiceLayout() (platform.ServiceLayout, error) {
	layout, err := h.Host.ServiceLayout()
	layout.Manager = "launchd"
	return layout, err
}

func TestLaunchdLifecycleAndLogs(t *testing.T) {
	t.Parallel()
	_, host, _ := subject(t)
	s := Service{Host: launchHost{host}, Config: "/home/test/custom config.toml"}
	options, err := s.Current("timer", DefaultInterval)
	must(t, err)
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	for _, name := range []string{"service.stdout.log", "service.stderr.log"} {
		path := filepath.Join(options.LogDir, name)
		info, statErr := host.Lstat(path)
		must(t, statErr)
		if info.Mode().Perm() != privatePerm {
			t.Fatal(info.Mode())
		}
		must(t, host.WriteAtomic(path, []byte("retain existing logs"), privatePerm))
	}
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	report, err := s.Status(t.Context())
	must(t, err)
	if report.Stale || !report.Installed {
		t.Fatal(report)
	}
	must(t, s.Uninstall(t.Context()))
	data, err := host.ReadFile(filepath.Join(options.LogDir, "service.stdout.log"))
	must(t, err)
	if string(data) != "retain existing logs" {
		t.Fatal("logs removed or overwritten")
	}
}

func TestFailedAtomicUpdatePreservesUnit(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	layout, err := host.ServiceLayout()
	must(t, err)
	path := filepath.Join(layout.Directory, platform.ServiceLabel+".service")
	before, err := host.ReadFile(path)
	must(t, err)
	must(t, host.InterruptNextWrite(path, fake.BeforeRename))
	options.Mode = "timer"
	if _, err = s.Install(t.Context(), options, false); err == nil {
		t.Fatal("ignored atomic write failure")
	}
	after, err := host.ReadFile(path)
	must(t, err)
	if string(after) != string(before) {
		t.Fatal("old unit was partially replaced")
	}
}

type brokenSupervisor struct{ platform.Host }

// ServiceControl simulates a failed supervisor operation.
func (brokenSupervisor) ServiceControl(context.Context, string, bool) error { return fs.ErrPermission }

func TestSupervisorFailures(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	s.Host = brokenSupervisor{host}
	if _, err := s.Install(t.Context(), options, false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	if err := s.Uninstall(t.Context()); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}

func TestRemovedUnitIsStale(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	layout, err := host.ServiceLayout()
	must(t, err)
	must(t, host.Remove(filepath.Join(layout.Directory, platform.ServiceLabel+".service")))
	report, err := s.Status(t.Context())
	must(t, err)
	if !report.Installed || !report.Stale {
		t.Fatal("missing installed unit was treated as optional absence", report)
	}
}

func executable(t *testing.T, host *fake.Host, path string, perm fs.FileMode) {
	t.Helper()
	must(t, host.MkdirAll(filepath.Dir(path), 0o755))
	must(t, host.WriteAtomic(path, nil, perm))
}

func TestLaunchdPathKeepsOnlyGitAndSSH(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, shell, want string }{
		{"tool-directories-first", "/home/test/.npm/bin:relative:/opt/homebrew/bin/:/usr/local/bin:/home/test/secret project/bin:/usr/bin:/bin", "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"},
		{"system-tools-only", "/usr/bin:/home/test/.cargo/bin", "/usr/bin:/bin:/usr/sbin:/sbin"},
		{"no-shell-path", "", "/usr/bin:/bin:/usr/sbin:/sbin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "PATH": tc.shell}), fake.WithServiceLayout(platform.ServiceLayout{Manager: "launchd", Directory: "/home/test/Library/LaunchAgents", Binary: "/opt/homebrew/opt/kitbag/bin/kitbag"}))
			// A non-executable git earlier in PATH is not what the daemon would run.
			executable(t, host, "/home/test/.npm/bin/git", 0o644)
			executable(t, host, "/home/test/.npm/bin/node", 0o755)
			executable(t, host, "/opt/homebrew/bin/git", 0o755)
			executable(t, host, "/usr/local/bin/ssh", 0o755)
			executable(t, host, "/usr/bin/git", 0o755)
			executable(t, host, "/usr/bin/ssh", 0o755)
			options, err := Service{Host: host}.Current("daemon", DefaultInterval)
			must(t, err)
			if got := options.Environment["PATH"]; got != tc.want {
				t.Fatalf("PATH %q, want %q", got, tc.want)
			}
			definitions, err := Generate("launchd", options)
			must(t, err)
			if plist := definitions[platform.ServiceLabel+".plist"]; !strings.Contains(plist, "<key>PATH</key><string>"+tc.want+"</string>") || strings.Contains(plist, ".npm") {
				t.Fatal(plist)
			}
		})
	}
}

func TestSystemdKeepsShellPath(t *testing.T) {
	t.Parallel()
	_, _, options := subject(t)
	if got := options.Environment["PATH"]; got != "/usr/bin:/bin" {
		t.Fatal("systemd PATH changed", got)
	}
}

func TestMissingBinaryIsReported(t *testing.T) {
	t.Parallel()
	// Homebrew's stable links point into a versioned keg: opt links the keg
	// directory, bin links the executable itself.
	for _, tc := range []struct{ name, binary, link, target string }{
		{"opt-directory-link", "/opt/homebrew/opt/kitbag/bin/kitbag", "/opt/homebrew/opt/kitbag", "../Cellar/kitbag/0.1.0"},
		{"bin-executable-link", "/opt/homebrew/bin/kitbag", "/opt/homebrew/bin/kitbag", "../Cellar/kitbag/0.1.0/bin/kitbag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"}), fake.WithServiceLayout(platform.ServiceLayout{Manager: "launchd", Directory: "/home/test/Library/LaunchAgents", Binary: tc.binary}))
			executable(t, host, "/opt/homebrew/Cellar/kitbag/0.1.0/bin/kitbag", 0o755)
			must(t, host.MkdirAll(filepath.Dir(tc.link), 0o755))
			must(t, host.Link(tc.target, tc.link))
			checkMissingBinary(t, Service{Host: host}, host, tc.binary)
		})
	}
}

func checkMissingBinary(t *testing.T, s Service, host *fake.Host, binary string) {
	t.Helper()
	options, err := s.Current("daemon", DefaultInterval)
	must(t, err)
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	report, err := s.Status(t.Context())
	must(t, err)
	if report.BinaryMissing || report.Binary != binary || report.Stale {
		t.Fatal(report)
	}
	// brew uninstall (or a cleanup of the only keg) leaves the link dangling.
	must(t, host.Remove("/opt/homebrew/Cellar/kitbag/0.1.0/bin/kitbag"))
	report, err = s.Status(t.Context())
	must(t, err)
	if !report.BinaryMissing || report.Binary != binary {
		t.Fatal(report)
	}
	data, err := json.Marshal(report)
	must(t, err)
	if !strings.Contains(string(data), `"binary":"`+binary+`"`) || !strings.Contains(string(data), `"binary_missing":true`) {
		t.Fatal(string(data))
	}
}
