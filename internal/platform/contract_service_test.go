package platform_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func serviceContract(t *testing.T, h platform.Service) {
	t.Helper()
	layout, err := h.ServiceLayout()
	mustNil(t, "layout", err)
	if layout.Binary == "" || layout.Directory == "" {
		t.Fatal(layout)
	}
	mustNil(t, "enable", h.ServiceControl(t.Context(), "enable", false))
	mustNil(t, "start", h.ServiceControl(t.Context(), "start", false))
	status, err := h.ServiceInspect(t.Context(), false)
	mustNil(t, "inspect", err)
	if !status.Loaded || !status.Active || !status.Enabled {
		t.Fatal(status)
	}
	// A job that has not exited since it started has no exit code to report.
	if status.ExitCode != nil {
		t.Fatal("exit code for a job that never exited", *status.ExitCode)
	}
	mustNil(t, "stop", h.ServiceControl(t.Context(), "stop", false))
	mustNil(t, "disable", h.ServiceControl(t.Context(), "disable", false))
	mustNil(t, "reload", h.ServiceControl(t.Context(), "reload", false))
	if err = h.ServiceControl(t.Context(), "arbitrary-command", false); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	unitNameContract(t, h)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = h.ServiceControl(ctx, "start", false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// unitNameContract rejects every name that is not one plain path element
// before any manager is asked.
func unitNameContract(t *testing.T, h platform.Service) {
	t.Helper()
	for _, name := range []string{"", ".", "..", "-x", "a/b", `a\b`, "a b", "a\x00b"} {
		if _, err := h.UnitInspect(t.Context(), name); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("UnitInspect(%q) = %v", name, err)
		}
	}
}

func TestFakeServiceContract(t *testing.T) {
	t.Parallel()
	serviceContract(t, fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"})))
}

func TestFakeUnitInspect(t *testing.T) {
	t.Parallel()
	h := fake.New()
	h.SetUnit("old.timer", platform.UnitStatus{Loaded: true})
	if status, err := h.UnitInspect(t.Context(), "old.timer"); err != nil || !status.Loaded || status.Enabled {
		t.Fatal(status, err)
	}
	if status, err := h.UnitInspect(t.Context(), "unknown.timer"); err != nil || status != (platform.UnitStatus{}) {
		t.Fatal(status, err)
	}
}
