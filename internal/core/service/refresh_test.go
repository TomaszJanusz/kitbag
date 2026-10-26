package service

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func staleDefinition(t *testing.T, s Service) string {
	t.Helper()
	layout, err := s.Host.ServiceLayout()
	must(t, err)
	name := platform.ServiceLabel + ".service"
	if layout.Manager == "launchd" {
		name = platform.ServiceLabel + ".plist"
	}
	path := filepath.Join(layout.Directory, name)
	must(t, s.Host.WriteAtomic(path, []byte("old definition"), filePerm))
	return path
}

func TestRefreshDefinitionAndDryRun(t *testing.T) {
	t.Parallel()
	for _, manager := range []string{"systemd", "launchd"} {
		t.Run(manager, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			if manager == "launchd" {
				s.Host = launchHost{host}
				var currentErr error
				options, currentErr = s.Current("daemon", DefaultInterval)
				must(t, currentErr)
			}
			_, err := s.Install(t.Context(), options, false)
			must(t, err)
			checkRefreshDryRun(t, s, host)
			before := host.ServiceCalls()
			r, err := s.Refresh(t.Context(), RefreshRequest{})
			must(t, err)
			if !r.Changed {
				t.Fatal(r)
			}
			calls := host.ServiceCalls()[len(before):]
			want := []string{"reload timer=false", "restart timer=false"}
			if manager == "launchd" {
				want = []string{"reload timer=false", "stop timer=false", "start timer=false"}
			}
			if !slices.Equal(calls, want) {
				t.Fatal(calls)
			}
			before = host.ServiceCalls()
			r, err = s.Refresh(t.Context(), RefreshRequest{})
			must(t, err)
			if r.Changed || !slices.Equal(before, host.ServiceCalls()) {
				t.Fatal("repeat restarted", r)
			}
		})
	}
}

func TestRefreshVersionMismatchAndNegativeControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, mode, daemon string
		running, want      bool
	}{
		{"old daemon", "daemon", "0.1.1", true, true},
		{"same version", "daemon", "0.1.3", true, false},
		{"no version handshake", "daemon", "", false, true},
		{"timer", "timer", "0.1.1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			options.Mode = tc.mode
			_, err := s.Install(t.Context(), options, false)
			must(t, err)
			before := len(host.ServiceCalls())
			r, err := s.Refresh(t.Context(), RefreshRequest{DaemonVersion: tc.daemon, BinaryVersion: "0.1.3", Running: tc.running})
			must(t, err)
			if r.Changed != tc.want || r.DefinitionChanged || r.RestartNeeded != tc.want {
				t.Fatal(r)
			}
			if (len(host.ServiceCalls()) > before) != tc.want {
				t.Fatal(host.ServiceCalls())
			}
		})
	}
}

func TestRefreshAbsentAndInactive(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	r, err := s.Refresh(t.Context(), RefreshRequest{Running: true, DaemonVersion: "old", BinaryVersion: "new"})
	must(t, err)
	if r.Installed || r.Changed || len(host.ServiceCalls()) != 0 {
		t.Fatal(r)
	}
	path, err := s.metadata()
	must(t, err)
	if _, err = host.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	_, err = s.Install(t.Context(), options, true)
	must(t, err)
	staleDefinition(t, s)
	before := len(host.ServiceCalls())
	r, err = s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
	if !r.DefinitionChanged || r.RestartNeeded || !slices.Equal(host.ServiceCalls()[before:], []string{"reload timer=false"}) {
		t.Fatal(r, host.ServiceCalls())
	}
}

func TestRefreshAtomicFailureKeepsRunning(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	path := staleDefinition(t, s)
	must(t, host.InterruptNextWrite(path, fake.BeforeRename))
	before := host.ServiceCalls()
	_, err = s.Refresh(t.Context(), RefreshRequest{})
	if err == nil || !slices.Equal(before, host.ServiceCalls()) {
		t.Fatal("stopped before write", err, host.ServiceCalls())
	}
	data, err := host.ReadFile(path)
	must(t, err)
	if string(data) != "old definition" {
		t.Fatal(string(data))
	}
	_, err = s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
}

type reloadFailure struct{ platform.Host }

func (h reloadFailure) ServiceControl(ctx context.Context, op string, timer bool) error {
	if op == "reload" {
		return fs.ErrPermission
	}
	return h.Host.ServiceControl(ctx, op, timer)
}

func TestRefreshRetriesFailedReload(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	staleDefinition(t, s)
	broken := Service{Host: reloadFailure{host}}
	_, err = broken.Refresh(t.Context(), RefreshRequest{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	r, err := s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
	if !r.Changed || !r.RestartNeeded {
		t.Fatal("lost pending reload", r)
	}
}

func TestAutoRefreshGuardAndVersion(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	staleDefinition(t, s)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	restart, err := s.AutoRefresh(t.Context(), "0.1.3", now, nil)
	must(t, err)
	if !restart {
		t.Fatal("did not arrange restart")
	}
	staleDefinition(t, s)
	before := host.ServiceCalls()
	restart, err = s.AutoRefresh(t.Context(), "0.1.3", now.Add(time.Minute), nil)
	must(t, err)
	if restart || !slices.Equal(before, host.ServiceCalls()) {
		t.Fatal("refresh loop")
	}
	restart, err = s.AutoRefresh(t.Context(), "0.1.3", now.Add(AutoRefreshInterval), nil)
	must(t, err)
	if !restart {
		t.Fatal("backoff never expired")
	}
	staleDefinition(t, s)
	restart, err = s.AutoRefresh(t.Context(), "0.1.4", now.Add(AutoRefreshInterval), nil)
	must(t, err)
	if !restart {
		t.Fatal("new version suppressed")
	}
}

func TestAutoRefreshLaunchdDelegatesAndGuardsFailure(t *testing.T) {
	t.Parallel()
	_, host, _ := subject(t)
	s := Service{Host: launchHost{host}}
	options, err := s.Current("daemon", DefaultInterval)
	must(t, err)
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	path := staleDefinition(t, s)
	before := host.ServiceCalls()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	calls := 0
	detach := func() error { calls++; return fs.ErrPermission }
	restart, err := s.AutoRefresh(t.Context(), "new", now, detach)
	if restart || !errors.Is(err, fs.ErrPermission) || calls != 1 {
		t.Fatal(restart, err, calls)
	}
	_, err = s.AutoRefresh(t.Context(), "new", now.Add(time.Minute), detach)
	must(t, err)
	data, err := host.ReadFile(path)
	must(t, err)
	if calls != 1 || string(data) != "old definition" || !slices.Equal(before, host.ServiceCalls()) {
		t.Fatal("parent touched launchd", calls, host.ServiceCalls())
	}
}

func TestAutoRefreshOnlyOwnDaemon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode, config string
		noStart      bool
	}{
		{"timer", "", false}, {"daemon", "/other/config.toml", false}, {"daemon", "", true},
	} {
		t.Run(tc.mode+tc.config+fmtBool(tc.noStart), func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			options.Mode = tc.mode
			_, err := s.Install(t.Context(), options, tc.noStart)
			must(t, err)
			staleDefinition(t, s)
			s.Config = tc.config
			before := host.ServiceCalls()
			restart, err := s.AutoRefresh(t.Context(), "new", time.Now(), nil)
			must(t, err)
			if restart || !slices.Equal(before, host.ServiceCalls()) {
				t.Fatal("repaired another service")
			}
		})
	}
}

func fmtBool(value bool) string {
	if value {
		return "inactive"
	}
	return "active"
}

func TestRefreshPreservesInputsAndDetectsNewBinary(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	options.Config = "/custom/config.toml"
	options.Environment["PATH"] = "/custom/bin:/usr/bin"
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	s.Host = changedBinary{host}
	r, err := s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
	got, err := s.saved()
	must(t, err)
	if !r.DefinitionChanged || got.Binary != "/new/kitbag" || got.Config != options.Config || got.Environment["PATH"] != options.Environment["PATH"] {
		t.Fatal(r, got)
	}
}

type changedBinary struct{ platform.Host }

func (h changedBinary) ServiceLayout() (platform.ServiceLayout, error) {
	layout, err := h.Host.ServiceLayout()
	layout.Binary = "/new/kitbag"
	return layout, err
}

func TestDefinitionComparison(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		manager, a, b string
		same          bool
	}{
		{"systemd", "# comment\n[Service]\n ExecStart=\"hello world\" \n", "[Service]\nExecStart=\"hello world\"\n", true},
		{"systemd", "ExecStart=\"hello world\"", "ExecStart=\"helloworld\"", false},
		{"systemd", "Restart=always", "Restart=no", false},
		{"launchd", "<plist><dict><key>A</key><true/><key>B</key><string>x y</string></dict></plist>", "<plist>\n<dict><!-- note --><key>B</key><string>x y</string><key>A</key><true/></dict></plist>", true},
		{"launchd", "<array><string>a</string><string>b</string></array>", "<array><string>b</string><string>a</string></array>", false},
		{"launchd", "invalid", "other", false},
	} {
		t.Run(tc.manager+strings.ReplaceAll(tc.a, "/", "_"), func(t *testing.T) {
			t.Parallel()
			if got := equivalent(tc.manager, []byte(tc.a), []byte(tc.b)); got != tc.same {
				t.Fatal(got, tc)
			}
		})
	}
}

func checkRefreshDryRun(t *testing.T, s Service, host *fake.Host) {
	t.Helper()
	path := staleDefinition(t, s)
	before := host.ServiceCalls()
	r, err := s.Refresh(t.Context(), RefreshRequest{DryRun: true})
	must(t, err)
	if !r.Changed || !r.DefinitionChanged || !r.RestartNeeded || !slices.Equal(before, host.ServiceCalls()) {
		t.Fatal(r, host.ServiceCalls())
	}
	data, err := host.ReadFile(path)
	must(t, err)
	if string(data) != "old definition" {
		t.Fatal("dry run wrote definition")
	}
}

type startFailure struct{ platform.Host }

func (h startFailure) ServiceControl(ctx context.Context, op string, timer bool) error {
	if op == "start" {
		return fs.ErrPermission
	}
	return h.Host.ServiceControl(ctx, op, timer)
}

func TestRefreshResumesBootstrapWithoutFakingStatus(t *testing.T) {
	t.Parallel()
	_, host, _ := subject(t)
	s := Service{Host: launchHost{host}}
	options, err := s.Current("daemon", DefaultInterval)
	must(t, err)
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	staleDefinition(t, s)
	broken := Service{Host: startFailure{s.Host}}
	_, err = broken.Refresh(t.Context(), RefreshRequest{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
	actual, err := s.Status(t.Context())
	must(t, err)
	if actual.Active || actual.Loaded || !actual.Stale {
		t.Fatal("status invented a running service", actual)
	}
	r, err := s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
	if !r.Changed || !r.RestartNeeded {
		t.Fatal(r)
	}
	actual, err = s.Status(t.Context())
	must(t, err)
	if !actual.Active || !actual.Loaded || actual.Stale {
		t.Fatal(actual)
	}
}

func TestAutoRefreshGuardRemembersVersionsAndClockRollback(t *testing.T) {
	t.Parallel()
	s, host, _ := subject(t)
	dirs, err := host.Dirs()
	must(t, err)
	must(t, host.MkdirAll(dirs.State, directoryPerm))
	path := filepath.Join(dirs.State, "guard.json")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, version := range []string{"a", "b"} {
		allowed, guardErr := s.allowAutoRefresh(path, version, now)
		must(t, guardErr)
		if !allowed {
			t.Fatal(version)
		}
	}
	for _, at := range []time.Time{now.Add(time.Minute), now.Add(-time.Minute)} {
		allowed, guardErr := s.allowAutoRefresh(path, "a", at)
		must(t, guardErr)
		if allowed {
			t.Fatal("version switch or clock rollback bypassed guard")
		}
	}
}

func TestAutoRefreshFailedReloadStaysUsableAndExplicitBypassesGuard(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	staleDefinition(t, s)
	broken := Service{Host: reloadFailure{host}}
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	restart, err := broken.AutoRefresh(t.Context(), "new", now, nil)
	if restart || !errors.Is(err, fs.ErrPermission) {
		t.Fatal(restart, err)
	}
	actual, err := s.Status(t.Context())
	must(t, err)
	if !actual.Active || !actual.Stale {
		t.Fatal(actual)
	}
	restart, err = s.AutoRefresh(t.Context(), "new", now.Add(time.Minute), nil)
	must(t, err)
	if restart {
		t.Fatal("failed reload looped")
	}
	r, err := s.Refresh(t.Context(), RefreshRequest{})
	must(t, err)
	if !r.RestartNeeded {
		t.Fatal("explicit refresh failed to finish pending restart", r)
	}
}
