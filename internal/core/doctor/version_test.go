package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
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
