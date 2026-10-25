package unix

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestLaunchdStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data      string
		code            int
		wantErr, loaded bool
	}{
		{"missing", "", 113, false, false},
		{"active", "state = running\nlast exit code = 7", 0, false, true},
		{"bad-exit", "last exit code = invalid", 0, true, true},
		{"domain-failure", "", 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := New(WithServiceRunner(func(_ context.Context, _ string, args, _ []string) (string, int, error) {
				if args[0] == "print-disabled" {
					return "", 0, nil
				}
				return tc.data, tc.code, nil
			}))
			status, err := h.ServiceInspect(t.Context(), true)
			if (err != nil) != tc.wantErr || status.Loaded != tc.loaded {
				t.Fatal(status, err)
			}
			if tc.name == "active" && (status.ExitCode == nil || *status.ExitCode != 7 || !status.Active) {
				t.Fatal(status)
			}
		})
	}
}

func TestLaunchdCommands(t *testing.T) {
	t.Parallel()
	var calls []string
	loaded := false
	h := New(WithEnv(func(key string) (string, bool) {
		return map[string]string{"HOME": "/tmp/test", "USER": "testing"}[key], true
	}), WithServiceRunner(func(_ context.Context, name string, args, _ []string) (string, int, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if args[0] == "print" && !loaded {
			return "", 113, nil
		}
		if args[0] == "bootstrap" {
			loaded = true
		}
		return "state = running", 0, nil
	}))
	for _, operation := range []string{"reload", "enable", "start", "start", "stop", "disable"} {
		if err := h.ServiceControl(t.Context(), operation, false); err != nil {
			t.Fatal(err)
		}
	}
	text := strings.Join(calls, "\n")
	for _, fragment := range []string{"launchctl enable gui/", "launchctl bootstrap gui/", "/tmp/test/Library/LaunchAgents/dev.kitbag.agent.plist", "launchctl kickstart gui/", "launchctl bootout gui/", "launchctl disable gui/"} {
		if !strings.Contains(text, fragment) {
			t.Fatal(text)
		}
	}
	h.serviceRun = func(context.Context, string, []string, []string) (string, int, error) { return "", 0, fs.ErrPermission }
	if _, err := h.ServiceInspect(t.Context(), false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}
