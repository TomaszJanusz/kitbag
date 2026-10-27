package doctor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
)

func TestDaemonVersionWarningAndNegativeControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, version string
		err           error
		want          Status
		running       bool
	}{
		{"mismatch", "0.1.1", nil, Warn, true},
		{"matching", "0.1.3", nil, OK, true},
		{"absent", "", nil, OK, false},
		{"unreadable", "", errors.New("unauthorized"), Warn, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := subject(t)
			s.Version = "0.1.3"
			s.DaemonVersion = func(context.Context) (string, bool, error) { return tc.version, tc.running, tc.err }
			checks := s.daemonVersionCheck(t.Context())
			if !tc.running && tc.err == nil {
				if len(checks) != 0 {
					t.Fatal(checks)
				}
				return
			}
			if len(checks) != 1 || checks[0].Status != tc.want {
				t.Fatal(checks)
			}
			assertDaemonVersionMessage(t, checks[0], tc.version, tc.running)
			if tc.name == "mismatch" && checks[0].Fix != "run kitbag service refresh" {
				t.Fatal(checks)
			}
		})
	}
}

func assertDaemonVersionMessage(t *testing.T, check Check, version string, running bool) {
	t.Helper()
	if running && !strings.Contains(check.Message, "daemon "+version+", binary 0.1.3") {
		t.Fatal(check)
	}
}

func TestDoctorRecoversUpgradeWithBoundedNotice(t *testing.T) {
	t.Parallel()
	for _, matching := range []bool{false, true} {
		t.Run(fmtBool(matching), func(t *testing.T) {
			t.Parallel()
			s, host := subject(t)
			s.Version = "0.1.3"
			version := "0.1.2"
			if matching {
				version = s.Version
			}
			s.DaemonVersion = func(context.Context) (string, bool, error) { return version, true, nil }
			now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			s.Now = func() time.Time { return now }
			supervisor := service.Service{Host: host}
			options, err := supervisor.Current("daemon", service.DefaultInterval)
			must(t, err)
			must(t, host.MkdirAll(filepath.Dir(options.Binary), 0o755))
			must(t, host.WriteAtomic(options.Binary, []byte("binary"), 0o755))
			_, err = supervisor.Install(t.Context(), options, false)
			must(t, err)
			before := len(host.ServiceCalls())
			checks := s.daemonVersionCheck(t.Context())
			pending := strings.Contains(checks[0].Message, "restarting on the new version")
			if pending == matching || checks[0].Status != OK {
				t.Fatal(checks)
			}
			if (len(host.ServiceCalls()) > before) == matching {
				t.Fatal("incorrect supervisor actions", host.ServiceCalls())
			}
			calls := len(host.ServiceCalls())
			now = now.Add(service.RecoveryWindow + time.Second)
			checks = s.daemonVersionCheck(t.Context())
			if len(host.ServiceCalls()) != calls {
				t.Fatal("repeated recovery")
			}
			if !matching && (checks[0].Status != Warn || checks[0].Fix != "run kitbag service refresh") {
				t.Fatal("persistent mismatch hidden", checks)
			}
		})
	}
}

func fmtBool(matching bool) string {
	if matching {
		return "identical reinstall"
	}
	return "upgrade"
}
