package doctor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestServiceHealth(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	check := find(t, s.Run(t.Context()), "service")
	if check.Status != OK {
		t.Fatal(check)
	}
	supervisor := service.Service{Host: host}
	options, err := supervisor.Current("daemon", service.DefaultInterval)
	must(t, err)
	must(t, host.MkdirAll(filepath.Dir(options.Binary), 0o755))
	must(t, host.WriteAtomic(options.Binary, nil, 0o755))
	_, err = supervisor.Install(t.Context(), options, true)
	must(t, err)
	if check = find(t, s.Run(t.Context()), "service"); check.Status != Warn || check.Fix == "" {
		t.Fatal(check)
	}
	_, err = supervisor.Install(t.Context(), options, false)
	must(t, err)
	if check = find(t, s.Run(t.Context()), "service"); check.Status != OK {
		t.Fatal(check)
	}
	// A package upgrade removed the executable the definition still runs.
	must(t, host.Remove(options.Binary))
	if check = find(t, s.Run(t.Context()), "service"); check.Status != Warn || !strings.Contains(check.Message, options.Binary+" no longer exists") || !strings.Contains(check.Fix, "kitbag service install") {
		t.Fatal(check)
	}
	must(t, host.WriteAtomic(options.Binary, nil, 0o755))
	layout, err := host.ServiceLayout()
	must(t, err)
	must(t, host.WriteAtomic(filepath.Join(layout.Directory, platform.ServiceLabel+".service"), []byte("stale"), 0o644))
	if check = find(t, s.Run(t.Context()), "service"); check.Status != Warn {
		t.Fatal(check)
	}
}

type failedServiceHost struct{ platform.Host }

// ServiceInspect simulates an active timer whose last oneshot failed.
func (failedServiceHost) ServiceInspect(context.Context, bool) (platform.ServiceStatus, error) {
	code := 1
	return platform.ServiceStatus{Loaded: true, Active: true, Enabled: true, ExitCode: &code}, nil
}

func TestFailedTimerWarns(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	supervisor := service.Service{Host: host}
	options, err := supervisor.Current("timer", service.DefaultInterval)
	must(t, err)
	_, err = supervisor.Install(t.Context(), options, false)
	must(t, err)
	s.Host = failedServiceHost{host}
	if check := find(t, s.Run(t.Context()), "service"); check.Status != Warn {
		t.Fatal(check)
	}
}
