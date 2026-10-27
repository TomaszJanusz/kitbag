package service

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func recoveryExecutable(t *testing.T, s Service, options Options) {
	t.Helper()
	must(t, s.Host.MkdirAll(filepath.Dir(options.Binary), privatePerm|0o100))
	must(t, s.Host.WriteAtomic(options.Binary, []byte("installed binary"), 0o755))
}

func TestRecoverVersionOnceAndBounded(t *testing.T) {
	t.Parallel()
	for _, manager := range []string{"systemd", "launchd"} {
		t.Run(manager, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			if manager == "launchd" {
				s.Host = launchHost{host}
				var err error
				options, err = s.Current("daemon", DefaultInterval)
				must(t, err)
			}
			recoveryExecutable(t, s, options)
			_, err := s.Install(t.Context(), options, false)
			must(t, err)
			request := RefreshRequest{DaemonVersion: "0.1.2", BinaryVersion: "0.1.3", Running: true}
			now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			before := len(host.ServiceCalls())
			pending, err := s.RecoverVersion(t.Context(), request, now)
			must(t, err)
			if !pending || len(host.ServiceCalls()) <= before {
				t.Fatal("upgrade not restarted", pending, host.ServiceCalls())
			}
			calls := len(host.ServiceCalls())
			// A fresh client uses the same durable marker, including after the notice expires.
			for _, elapsed := range []time.Duration{time.Second, RecoveryWindow - time.Second, RecoveryWindow + time.Second} {
				pending, err = (Service{Host: s.Host}).RecoverVersion(t.Context(), request, now.Add(elapsed))
				must(t, err)
				if pending != (elapsed < RecoveryWindow) || len(host.ServiceCalls()) != calls {
					t.Fatal("restart loop or unbounded notice", elapsed, pending, host.ServiceCalls())
				}
			}
		})
	}
}

func TestRecoverVersionPolicyControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, next, mode string
		noStart               bool
	}{
		{name: "same content reinstall", old: "0.1.3", next: "0.1.3", mode: "daemon"},
		{name: "older client", old: "0.1.4", next: "0.1.3", mode: "daemon"},
		{name: "invalid version", old: "untrusted", next: "0.1.3", mode: "daemon"},
		{name: "timer", old: "0.1.2", next: "0.1.3", mode: "timer"},
		{name: "stopped", old: "0.1.2", next: "0.1.3", mode: "daemon", noStart: true},
		{name: "other config", old: "0.1.2", next: "0.1.3", mode: "daemon"},
		{name: "disabled", old: "0.1.2", next: "0.1.3", mode: "daemon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			options.Mode = tc.mode
			recoveryExecutable(t, s, options)
			_, err := s.Install(t.Context(), options, tc.noStart)
			must(t, err)
			if tc.name == "other config" {
				s.Config = "/home/test/other.toml"
			}
			if tc.name == "disabled" {
				must(t, host.ServiceControl(t.Context(), "disable", false))
			}
			before := len(host.ServiceCalls())
			pending, err := s.RecoverVersion(t.Context(), RefreshRequest{DaemonVersion: tc.old, BinaryVersion: tc.next, Running: true}, time.Now())
			must(t, err)
			if pending || len(host.ServiceCalls()) != before {
				t.Fatal("unwanted recovery", pending, host.ServiceCalls())
			}
			assertNoRecoveryAttempt(t, host)
		})
	}
}

func TestRecoverVersionFailureIsNotTransientOrRepeated(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	recoveryExecutable(t, s, options)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	s.Host = reloadFailure{host}
	request := RefreshRequest{DaemonVersion: "0.1.2", BinaryVersion: "0.1.3", Running: true}
	now := time.Now()
	pending, err := s.RecoverVersion(t.Context(), request, now)
	if pending || err == nil {
		t.Fatal(pending, err)
	}
	before := len(host.ServiceCalls())
	pending, err = s.RecoverVersion(t.Context(), request, now.Add(time.Second))
	if pending || err == nil || !strings.Contains(err.Error(), "previous automatic") || len(host.ServiceCalls()) != before {
		t.Fatal("failed recovery hidden or repeated", pending, err, host.ServiceCalls())
	}
}

type exitedLaunchd struct{ platform.Host }

func (h exitedLaunchd) ServiceInspect(ctx context.Context, timer bool) (platform.ServiceStatus, error) {
	status, err := h.Host.ServiceInspect(ctx, timer)
	code := RestartExitCode
	status.Active, status.Loaded, status.ExitCode = false, true, &code
	return status, err
}

func TestRecoverVersionRepairsRemovedLoadedCellarPath(t *testing.T) {
	t.Parallel()
	s, host, _ := subject(t)
	s.Host = launchHost{host}
	options, err := s.Current("daemon", DefaultInterval)
	must(t, err)
	recoveryExecutable(t, s, options)
	options.Binary = "/removed/Cellar/kitbag/0.1.2/bin/kitbag"
	_, err = s.Install(t.Context(), options, false)
	must(t, err)
	s.Host = exitedLaunchd{s.Host}
	before := len(host.ServiceCalls())
	pending, err := s.RecoverVersion(t.Context(), RefreshRequest{BinaryVersion: "0.1.3"}, time.Now())
	must(t, err)
	if !pending || len(host.ServiceCalls()) != before+3 {
		t.Fatal(pending, host.ServiceCalls())
	}
	report, err := s.Status(t.Context())
	must(t, err)
	if report.Stale || report.BinaryMissing || strings.Contains(report.Binary, "Cellar") {
		t.Fatal(report)
	}
}

func assertNoRecoveryAttempt(t *testing.T, host platform.Host) {
	t.Helper()
	dirs, err := host.Dirs()
	must(t, err)
	_, err = host.ReadFile(filepath.Join(dirs.State, "service-version-recovery.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("ineligible observation recorded a recovery attempt", err)
	}
}

func TestRecoverVersionRejectsUnusableInstalledExecutable(t *testing.T) {
	t.Parallel()
	for _, present := range []bool{false, true} {
		name := "missing"
		if present {
			name = "not executable"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, host, options := subject(t)
			if present {
				must(t, host.MkdirAll(filepath.Dir(options.Binary), 0o700))
				must(t, host.WriteAtomic(options.Binary, []byte("binary"), 0o600))
			}
			_, err := s.Install(t.Context(), options, false)
			must(t, err)
			before := len(host.ServiceCalls())
			pending, err := s.RecoverVersion(t.Context(), RefreshRequest{DaemonVersion: "0.1.2", BinaryVersion: "0.1.3", Running: true}, time.Now())
			if pending || err == nil || len(host.ServiceCalls()) != before {
				t.Fatal("unusable binary restarted", pending, err, host.ServiceCalls())
			}
			assertNoRecoveryAttempt(t, host)
		})
	}
}
