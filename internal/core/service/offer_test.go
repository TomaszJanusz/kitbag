package service

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestDecideNeverInstallsSilently(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		want                          Offer
		install, decline, interactive bool
	}{
		{want: OfferHint},
		{want: OfferAsk, interactive: true},
		{want: OfferInstall, install: true},
		{want: OfferInstall, install: true, interactive: true},
		{want: OfferDecline, decline: true},
		{want: OfferDecline, decline: true, interactive: true},
	} {
		if got := Decide(tc.install, tc.decline, tc.interactive); got != tc.want {
			t.Errorf("Decide(%t, %t, %t) = %d, want %d", tc.install, tc.decline, tc.interactive, got, tc.want)
		}
	}
}

func TestInstallDefaultWritesStartedDaemonAndReportsPaths(t *testing.T) {
	t.Parallel()
	s, host, _ := subject(t)
	assertInstalled(t, s, false)
	installation, err := s.InstallDefault(t.Context())
	must(t, err)
	assertInstalled(t, s, true)
	layout, err := host.ServiceLayout()
	must(t, err)
	dirs, err := host.Dirs()
	must(t, err)
	want := []string{filepath.Join(layout.Directory, platform.ServiceLabel+".service"), filepath.Join(dirs.State, "service.json")}
	if !installation.Changed || installation.Mode != "daemon" || !slices.Equal(installation.Paths, want) {
		t.Fatalf("installation=%+v want paths %v", installation, want)
	}
	for _, path := range installation.Paths {
		_, err = host.Lstat(path)
		must(t, err)
	}
	assertActiveDaemon(t, s)
	again, err := s.InstallDefault(t.Context())
	must(t, err)
	if again.Changed || !slices.Equal(again.Paths, want) {
		t.Fatalf("repeat installation=%+v", again)
	}
}

func assertActiveDaemon(t *testing.T, s Service) {
	t.Helper()
	report, err := s.Status(t.Context())
	must(t, err)
	if !report.Active || !report.Enabled || report.Stale || report.Mode != "daemon" {
		t.Fatal(report)
	}
}

func assertInstalled(t *testing.T, s Service, want bool) {
	t.Helper()
	installed, err := s.Installed()
	must(t, err)
	if installed != want {
		t.Fatalf("installed=%t want %t", installed, want)
	}
}

type unsupportedServiceHost struct{ platform.Host }

func (unsupportedServiceHost) ServiceLayout() (platform.ServiceLayout, error) {
	return platform.ServiceLayout{}, platform.ErrUnsupported
}

func TestInstallDefaultReportsUnsupportedHosts(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	s.Host = unsupportedServiceHost{host}
	if _, err := s.InstallDefault(t.Context()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := s.Paths(options); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := s.Installed(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}
