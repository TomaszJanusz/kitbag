package doctor

import (
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func TestDaemonPollHealth(t *testing.T) {
	t.Parallel()
	s, host := subject(t)
	now := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	supervisor := service.Service{Host: host}
	options, err := supervisor.Current("daemon", service.DefaultInterval)
	must(t, err)
	_, err = supervisor.Install(t.Context(), options, false)
	must(t, err)
	r := state.CheckRecord{SchemaVersion: 1, At: now.Add(-3 * time.Minute), NextAt: now.Add(-2 * time.Minute), Repository: s.Repository, Outcome: "ok", IntervalSeconds: 60}
	must(t, s.Store.SaveCheck(r))
	if check := find(t, s.Run(t.Context()), "daemon-poll"); check.Status != OK {
		t.Fatal(check)
	}
	now = now.Add(time.Second)
	if check := find(t, s.Run(t.Context()), "daemon-poll"); check.Status != Warn || check.Fix == "" {
		t.Fatal(check)
	}
}
