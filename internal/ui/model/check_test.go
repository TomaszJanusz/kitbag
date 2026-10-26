package model

import (
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
)

func TestCheckWarningFreshness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC)
	m := readyStatus(t, &fakeClient{status: StatusSnapshot{Daemon: true, LastCheck: &LastCheck{At: now.Add(-time.Minute), NextAt: now, Outcome: "ok", Interval: time.Minute}}})
	m.clock = timefmt.Clock{Now: func() time.Time { return now }}
	expect(t, len(m.View().Warnings) == 0, "fresh check warned")
	now = now.Add(3 * time.Minute)
	expect(t, len(m.View().Warnings) == 1 && render(m.View().Warnings[0]) == "daemon is not checking", "stale check shown fresh: %+v", m.View())
	m.snapshot.LastCheck.Outcome, m.snapshot.LastCheck.Category = "failed", "operation"
	expect(t, len(m.View().Warnings) == 2 && strings.Contains(render(m.View().Warnings[0]), "failed: The operation failed"), "failed check hidden")
	m.snapshot.Daemon = false
	view := m.View()
	expect(t, len(view.Warnings) == 1, "stopped daemon claimed stuck")
	expect(t, strings.Contains(render(view.Rows[4].Value), "stale; daemon not running"), "stale check shown fresh")
	expect(t, len(view.Rows) == 7, "stopped daemon shows next poll")
}
