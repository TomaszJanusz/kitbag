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
	mustNil(t, "stop", h.ServiceControl(t.Context(), "stop", false))
	mustNil(t, "disable", h.ServiceControl(t.Context(), "disable", false))
	mustNil(t, "reload", h.ServiceControl(t.Context(), "reload", false))
	if err = h.ServiceControl(t.Context(), "arbitrary-command", false); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = h.ServiceControl(ctx, "start", false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFakeServiceContract(t *testing.T) {
	t.Parallel()
	serviceContract(t, fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test"})))
}
