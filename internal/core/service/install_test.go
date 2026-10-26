package service

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"
)

func TestInstallObservedRestartsOldProcessWithIdenticalDefinition(t *testing.T) {
	t.Parallel()
	for _, manager := range []string{"systemd", "launchd"} {
		t.Run(manager, func(t *testing.T) {
			t.Parallel()
			for _, version := range []string{"0.1.1", "older", ""} {
				s, host, _ := subject(t)
				if manager == "launchd" {
					s.Host = launchHost{host}
				}
				options, currentErr := s.Current("daemon", DefaultInterval)
				must(t, currentErr)
				_, err := s.Install(t.Context(), options, false)
				must(t, err)
				layout, err := s.Host.ServiceLayout()
				must(t, err)
				path := filepath.Join(layout.Directory, unitNames(manager)[0])
				before, err := host.ReadFile(path)
				must(t, err)
				calls := len(host.ServiceCalls())
				r, err := s.InstallObserved(t.Context(), options, false, RefreshRequest{DaemonVersion: version, BinaryVersion: "0.1.2", Running: version != ""})
				must(t, err)
				if r.Changed || !r.Restarted {
					t.Fatal("old process kept alive", r)
				}
				after, err := host.ReadFile(path)
				must(t, err)
				if !bytes.Equal(before, after) {
					t.Fatal("restart rewrote identical definition")
				}
				want := "restart timer=false"
				if manager == "launchd" {
					want = "stop timer=false"
				}
				if !slices.Contains(host.ServiceCalls()[calls:], want) {
					t.Fatal(host.ServiceCalls())
				}
				assertInstallMatchingDoesNotRestart(t, s, options, host.ServiceCalls)
			}
		})
	}
}

func assertInstallMatchingDoesNotRestart(t *testing.T, s Service, options Options, calls func() []string) {
	t.Helper()
	for range 2 {
		before := len(calls())
		r, err := s.InstallObserved(t.Context(), options, false, RefreshRequest{DaemonVersion: "0.1.2", BinaryVersion: "0.1.2", Running: true})
		must(t, err)
		if r.Changed || r.Restarted {
			t.Fatal("matching daemon refreshed again", r)
		}
		for _, call := range calls()[before:] {
			if call == "restart timer=false" || call == "stop timer=false" {
				t.Fatal("restart loop", calls())
			}
		}
	}
}

func TestInstallObservedNoStartPreservesOldProcess(t *testing.T) {
	t.Parallel()
	s, host, options := subject(t)
	_, err := s.Install(t.Context(), options, false)
	must(t, err)
	before := len(host.ServiceCalls())
	r, err := s.InstallObserved(t.Context(), options, true, RefreshRequest{DaemonVersion: "0.1.1", BinaryVersion: "0.1.2", Running: true})
	must(t, err)
	if r.Restarted || slices.Contains(host.ServiceCalls()[before:], "restart timer=false") {
		t.Fatal(r, host.ServiceCalls())
	}
}
