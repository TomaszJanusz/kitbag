package unix

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestLaunchdStatus(t *testing.T) {
	t.Parallel()
	const none = -1
	for _, tc := range []struct {
		name, data              string
		code, exit              int
		wantErr, loaded, active bool
	}{
		{"missing", "", 113, none, false, false, false},
		{"active", "state = running\nlast exit code = 7", 0, 7, false, true, true},
		{"bad-exit", "last exit code = invalid", 0, none, true, true, false},
		{"domain-failure", "", 1, none, true, false, false},
		// A healthy, still running job has no exit to report.
		{"never-exited", "state = running\n\tlast exit code = (never exited)", 0, none, false, true, true},
		{"described-exit", "state = waiting\n\tlast exit code = 78: EX_CONFIG", 0, 78, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := New(WithServiceRunner(func(_ context.Context, _ string, args, _ []string) (string, int, error) {
				if args[0] == "print-disabled" {
					return "", 0, nil
				}
				return tc.data, tc.code, nil
			}))
			status, err := h.ServiceInspect(t.Context(), false)
			if (err != nil) != tc.wantErr || status.Loaded != tc.loaded || err == nil && status.Active != tc.active {
				t.Fatal(status, err)
			}
			if err == nil && !sameExit(status.ExitCode, tc.exit, none) {
				t.Fatal(tc.name, status)
			}
		})
	}
}

// sameExit compares an observed exit code with want, where none means no observation.
func sameExit(exit *int, want, none int) bool {
	if exit == nil {
		return want == none
	}
	return *exit == want
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

func TestLaunchdLayoutUsesStableHomebrewPath(t *testing.T) {
	t.Parallel()
	root := homebrewTree(t, "opt", "bin")
	keg := filepath.Join(root, "Cellar", "kitbag", "0.1.0", "bin", "kitbag")
	h := New(WithEnv(func(key string) (string, bool) {
		return map[string]string{"HOME": "/tmp/test", "USER": "testing"}[key], true
	}), WithExecutable(func() (string, error) { return keg, nil }))
	layout, err := h.ServiceLayout()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "opt", "kitbag", "bin", "kitbag"); layout.Binary != want {
		t.Fatalf("binary %s, want %s", layout.Binary, want)
	}
	h = New(WithEnv(func(string) (string, bool) { return "/tmp/test", true }), WithExecutable(func() (string, error) { return "", fs.ErrNotExist }))
	if _, err = h.ServiceLayout(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestLaunchdUnitInspect(t *testing.T) {
	t.Parallel()
	const label = "com.example.agent-config-sync"
	for _, tc := range []struct {
		name, overrides string
		print           int
		want            platform.UnitStatus
		wantErr         bool
	}{
		{"booted out and disabled", `"` + label + `" => disabled`, 113, platform.UnitStatus{}, false},
		{"older macOS disabled", `"` + label + `" => true`, 113, platform.UnitStatus{}, false},
		{"booted out, loads at login", "", 113, platform.UnitStatus{Enabled: true}, false},
		{"loaded and enabled", `"` + label + `" => enabled`, 0, platform.UnitStatus{Loaded: true, Enabled: true}, false},
		{"loaded but disabled", `"other" => enabled` + "\n" + `"` + label + `" => disabled`, 0, platform.UnitStatus{Loaded: true}, false},
		{"domain failure", "", 5, platform.UnitStatus{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := New(WithServiceRunner(func(_ context.Context, name string, args, _ []string) (string, int, error) {
				switch {
				case name != "launchctl":
					t.Errorf("unexpected program %s", name)
				case args[0] == "print-disabled":
					return tc.overrides, 0, nil
				case args[0] != "print" || !strings.HasSuffix(args[1], "/"+label):
					t.Errorf("unexpected command %v", args)
				}
				return "", tc.print, nil
			}))
			status, err := h.UnitInspect(t.Context(), label)
			if (err != nil) != tc.wantErr || status != tc.want {
				t.Fatal(status, err)
			}
		})
	}
	h := New(WithServiceRunner(func(context.Context, string, []string, []string) (string, int, error) {
		t.Error("an invalid label reached launchctl")
		return "", 0, nil
	}))
	for _, label := range []string{"", "-x", "user/501/x", "a\tb"} {
		if _, err := h.UnitInspect(t.Context(), label); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("%q: %v", label, err)
		}
	}
}
