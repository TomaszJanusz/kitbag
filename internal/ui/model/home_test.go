package model

import (
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestHomeProjectsCompactStatusAndCounts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	app := NewApp(10, timefmt.Clock{Now: func() time.Time { return now }})
	app.Init()
	app.Update(StatusLoaded{Seq: 1, Snapshot: StatusSnapshot{
		Repository: "/repo", Git: GitState{Branch: "main"}, Daemon: true,
		LastCheck: &LastCheck{At: now.Add(-2 * time.Minute), NextAt: now.Add(time.Minute), Outcome: "ok", Interval: time.Minute},
	}})
	home := app.Home("0.1.2")
	expect(t, render(home.Headline) == "Everything is in sync · checked 2 min ago", "headline=%s", render(home.Headline))
	expect(t, strings.Contains(render(home.Secondary), "Next check"), "secondary=%+v", home.Secondary)
	expect(t, len(home.Items) == 5 && home.Items[0].Action == HomeSync, "menu=%+v", home.Items)
	expect(t, render(home.Repository) == "/repo" && render(home.Branch) == "main", "edges=%+v", home)
	app.Update(Refresh{})
	app.Update(StatusLoaded{Seq: 2, Snapshot: StatusSnapshot{Drift: DriftCounts{LocalDrift: 2, UpdateNeeded: 1, Conflict: 1}}})
	home = app.Home("0.1.2")
	expect(t, render(home.Items[2].Value) == "4", "count=%+v", home.Items[2])
	expect(t, home.Level == Failure, "conflict lost severity")
	checkHomeText(t, home)
}

func checkHomeText(t *testing.T, view HomeView) {
	t.Helper()
	checkText(t, view.Headline, view.Secondary, view.Repository, view.Branch, view.Version)
	for _, item := range view.Items {
		checkText(t, item.Label, item.Shortcut, item.Value)
	}
}

func TestHomePreservesWarningsAndDisablesSync(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key      text.Key
		snapshot StatusSnapshot
	}{
		{text.StatusVersionMismatch, StatusSnapshot{Daemon: true, BinaryVersion: "2", DaemonVersion: "1", ServiceStale: true}},
		{text.StatusServiceStale, StatusSnapshot{ServiceStale: true}},
		{text.StatusCheckStopped, StatusSnapshot{LastCheck: &LastCheck{Outcome: "failed", Category: "busy"}}},
	} {
		app := NewApp(10, timefmt.Clock{})
		app.Init()
		app.Update(StatusLoaded{Seq: 1, Snapshot: tc.snapshot})
		view := app.Home("dev")
		expect(t, view.Secondary.Key == tc.key && view.SecondaryLevel == Warning, "warning=%+v", view.Secondary)
		checkHomeText(t, view)
	}
	app := NewApp(10, timefmt.Clock{})
	app.Init()
	app.Update(StatusLoaded{Seq: 1, Snapshot: StatusSnapshot{ToolTooOld: &ToolTooOld{Required: "2", Current: "1"}}})
	expect(t, !app.Home("1").Items[0].Enabled, "old binary can sync")
	app.Update(Refresh{})
	app.Update(StatusLoaded{Seq: 2, Err: &Error{Kind: ErrUnavailable, Detail: "raw transport detail"}})
	view := app.Home("1")
	expect(t, view.Level == Failure && !strings.Contains(render(view.Headline), "raw transport"), "unsafe headline=%+v", view)
}
