package unix

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestSystemdStatusAndLinger(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data      string
		code            int
		wantErr, loaded bool
	}{
		{"missing", "LoadState=not-found", 1, false, false},
		{"active", "LoadState=loaded\nActiveState=active\nUnitFileState=enabled\nExecMainStartTimestamp=Sun 2026-10-25 12:00:00 UTC\nExecMainStatus=7", 0, false, true},
		{"bad-exit", "LoadState=loaded\nExecMainStartTimestamp=now\nExecMainStatus=invalid", 0, true, true},
		{"bus-failure", "", 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := New(WithServiceRunner(func(context.Context, string, []string, []string) (string, int, error) { return tc.data, tc.code, nil }))
			status, err := h.ServiceInspect(t.Context(), true)
			if (err != nil) != tc.wantErr || status.Loaded != tc.loaded {
				t.Fatal(status, err)
			}
			if tc.name == "active" && (status.ExitCode == nil || *status.ExitCode != 7 || !status.Active || !status.Enabled) {
				t.Fatal(status)
			}
		})
	}
}

func TestSystemdCommands(t *testing.T) {
	t.Parallel()
	var calls []string
	h := New(WithEnv(func(key string) (string, bool) {
		return map[string]string{"HOME": "/tmp/test", "USER": "testing"}[key], true
	}), WithServiceRunner(func(_ context.Context, name string, args, _ []string) (string, int, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return "Linger=yes", 0, nil
	}))
	enabled, err := h.ServiceLinger(t.Context())
	if err != nil || !enabled {
		t.Fatal(enabled, err)
	}
	for _, operation := range []string{"reload", "enable", "start", "stop", "disable"} {
		if err = h.ServiceControl(t.Context(), operation, true); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"loginctl show-user testing -p Linger", "systemctl --user daemon-reload", "systemctl --user enable dev.kitbag.agent.timer", "systemctl --user start dev.kitbag.agent.timer", "systemctl --user stop dev.kitbag.agent.timer", "systemctl --user disable dev.kitbag.agent.timer"}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatal(calls)
	}
	h.serviceRun = func(context.Context, string, []string, []string) (string, int, error) { return "", 1, nil }
	if err = h.ServiceControl(t.Context(), "enable", false); err == nil {
		t.Fatal("ignored failure")
	}
	if _, err = h.ServiceLinger(t.Context()); err == nil {
		t.Fatal("ignored linger failure")
	}
	h.serviceRun = func(context.Context, string, []string, []string) (string, int, error) { return "", 0, fs.ErrPermission }
	if _, err = h.ServiceInspect(t.Context(), false); !errors.Is(err, fs.ErrPermission) {
		t.Fatal(err)
	}
}

func TestSystemdUnitInspect(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data string
		code       int
		want       platform.UnitStatus
		wantErr    bool
	}{
		{"unknown", "LoadState=not-found\nActiveState=inactive\nUnitFileState=", 0, platform.UnitStatus{}, false},
		{"disabled and stopped", "LoadState=loaded\nActiveState=inactive\nUnitFileState=disabled", 0, platform.UnitStatus{}, false},
		{"masked after failure", "LoadState=masked\nActiveState=failed\nUnitFileState=masked", 0, platform.UnitStatus{}, false},
		{"static service", "LoadState=loaded\nActiveState=inactive\nUnitFileState=static", 0, platform.UnitStatus{}, false},
		{"waiting timer", "LoadState=loaded\nActiveState=active\nUnitFileState=enabled", 0, platform.UnitStatus{Loaded: true, Enabled: true}, false},
		{"started by hand", "LoadState=loaded\nActiveState=activating\nUnitFileState=disabled", 0, platform.UnitStatus{Loaded: true}, false},
		{"unrecognized state is enabled", "LoadState=loaded\nActiveState=inactive\nUnitFileState=alias", 0, platform.UnitStatus{Enabled: true}, false},
		{"bus failure", "", 1, platform.UnitStatus{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := New(WithServiceRunner(func(_ context.Context, name string, args, _ []string) (string, int, error) {
				if name != "systemctl" || strings.Join(args, " ") != "--user show agent-config-sync.timer --property=LoadState,ActiveState,UnitFileState" {
					t.Errorf("unexpected command %s %v", name, args)
				}
				return tc.data, tc.code, nil
			}))
			status, err := h.UnitInspect(t.Context(), "agent-config-sync.timer")
			if (err != nil) != tc.wantErr || status != tc.want {
				t.Fatal(status, err)
			}
		})
	}
	h := New(WithServiceRunner(func(context.Context, string, []string, []string) (string, int, error) {
		t.Error("an invalid name reached systemctl")
		return "", 0, nil
	}))
	for _, name := range []string{"", "--all", "../x.timer", "a b.timer"} {
		if _, err := h.UnitInspect(t.Context(), name); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("%q: %v", name, err)
		}
	}
}
