package model

import (
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func readyStatus(t *testing.T, client *fakeClient) *Status {
	t.Helper()
	m := NewStatus()
	settle(t, m, client, m.Init())
	return m
}

func loadSeq(t *testing.T, cmds []Cmd) uint64 {
	t.Helper()
	return as[LoadStatus](t, only(t, cmds)).Seq
}

func TestStatusHeadlines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		want     text.Key
		snapshot StatusSnapshot
	}{
		{"in sync", text.StatusInSync, StatusSnapshot{Drift: DriftCounts{InSync: 3}}},
		{"updates", text.StatusUpdates, StatusSnapshot{Drift: DriftCounts{UpdateNeeded: 2}}},
		{"local drift", text.StatusLocalDrift, StatusSnapshot{Drift: DriftCounts{UpdateNeeded: 2, LocalDrift: 1}}},
		{"conflicts", text.StatusConflicts, StatusSnapshot{Drift: DriftCounts{LocalDrift: 1, Conflict: 1, UnmanagedObstacle: 1}}},
		{"tool too old", text.StatusToolTooOld, StatusSnapshot{ToolTooOld: &ToolTooOld{Required: "9.0.0", Current: "1.0.0"}}},
		{"schema too new", text.StatusSchemaTooNew, StatusSnapshot{ToolTooOld: &ToolTooOld{Current: "1.0.0", Schema: 2, SupportedSchema: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := readyStatus(t, &fakeClient{status: tc.snapshot}).View()
			expect(t, view.Phase == Ready, "phase=%v", view.Phase)
			expect(t, view.Headline.Key == tc.want, "headline=%+v", view.Headline)
			expect(t, view.Problem == nil, "problem=%+v", view.Problem)
			checkText(t, statusTexts(view)...)
		})
	}
}

func TestStatusLoadingAndFailure(t *testing.T) {
	t.Parallel()
	m := NewStatus()
	cmds := m.Init()
	view := m.View()
	expect(t, view.Phase == Loading && view.Headline.Key == text.StatusLoading, "loading=%+v", view)
	expect(t, len(view.Rows) == 0, "rows before a snapshot: %+v", view.Rows)
	settle(t, m, &fakeClient{statusErr: &Error{Kind: ErrUnavailable}}, cmds)
	view = m.View()
	expect(t, view.Phase == Failed && view.Headline.Key == text.ErrorUnavailable, "failed=%+v", view)
	expect(t, view.Problem != nil, "failure without a problem")
	checkText(t, statusTexts(view)...)
}

func TestStatusKeepsSnapshotWhenBusy(t *testing.T) {
	t.Parallel()
	m := readyStatus(t, &fakeClient{status: StatusSnapshot{Repository: "/repo", Daemon: true}})
	settle(t, m, &fakeClient{statusErr: &Error{Kind: ErrBusy}}, m.Update(Refresh{}))
	view := m.View()
	// Busy keeps the last snapshot visible and explains why it may be stale.
	expect(t, view.Phase == Ready, "phase=%v", view.Phase)
	expect(t, view.Problem != nil && view.Problem.Key == text.StatusStale, "problem=%+v", view.Problem)
	expect(t, view.Problem.Args[0].Key == text.ErrorBusy, "problem cause=%+v", view.Problem.Args)
	expect(t, render(view.Rows[0].Value) == "/repo", "repository row=%+v", view.Rows[0])
	checkText(t, statusTexts(view)...)
}

func TestStatusDiscardsOutOfOrderResults(t *testing.T) {
	t.Parallel()
	m := NewStatus()
	first := loadSeq(t, m.Init())
	second := loadSeq(t, m.Update(Refresh{}))
	m.Update(StatusLoaded{Seq: second, Snapshot: StatusSnapshot{Repository: "/new"}})
	m.Update(StatusLoaded{Seq: first, Snapshot: StatusSnapshot{Repository: "/old"}})
	m.Update(StatusLoaded{Seq: second + 1, Snapshot: StatusSnapshot{Repository: "/never-requested"}})
	m.Update(StatusLoaded{Seq: first, Err: errBoom})
	expect(t, m.snapshot.Repository == "/new", "stale result applied: %+v", m.snapshot)
	expect(t, m.problem == nil, "stale failure applied: %v", m.problem)
}

func TestStatusToolTooOldFromFailedLoad(t *testing.T) {
	t.Parallel()
	m := NewStatus()
	client := &fakeClient{status: StatusSnapshot{ToolTooOld: &ToolTooOld{Required: "2.0.0", Current: "1.0.0"}}, statusErr: &Error{Kind: ErrToolTooOld}}
	settle(t, m, client, m.Init())
	view := m.View()
	got := render(view.Headline)
	expect(t, got == "This repository requires kitbag 2.0.0 or newer; running 1.0.0", "headline=%q", got)
	expect(t, !action(t, view.Actions, SyncNow{}).Enabled, "sync offered for a repository requiring a newer tool")
	expect(t, m.Update(SyncNow{}) == nil, "sync started for a repository requiring a newer tool")
}

func TestStatusSchemaTooNewFromFailedLoad(t *testing.T) {
	t.Parallel()
	m := NewStatus()
	client := &fakeClient{status: StatusSnapshot{ToolTooOld: &ToolTooOld{Current: "1.0.0", Schema: 2, SupportedSchema: 1}}, statusErr: &Error{Kind: ErrToolTooOld}}
	settle(t, m, client, m.Init())
	view := m.View()
	got := render(view.Headline)
	expect(t, got == "This repository uses manifest schema 2; running kitbag 1.0.0, which reads schema 1", "headline=%q", got)
	expect(t, m.Update(SyncNow{}) == nil, "sync started for a manifest schema this tool cannot read")
}

func TestStatusSyncLifecycle(t *testing.T) {
	t.Parallel()
	client := &fakeClient{status: StatusSnapshot{Drift: DriftCounts{UpdateNeeded: 1}}}
	m := readyStatus(t, client)
	expect(t, only(t, m.Update(SyncNow{})) == (RunSync{}), "SyncNow did not run a sync")
	expect(t, m.Update(SyncNow{}) == nil, "a second SyncNow started another run")
	m.Update(SyncProgressed{Phase: "apply"})
	view := m.View()
	expect(t, view.Syncing, "not syncing")
	expect(t, render(view.Headline) == "Syncing: applying changes", "headline=%q", render(view.Headline))
	expect(t, !action(t, view.Actions, SyncNow{}).Enabled, "sync offered while syncing")
	client.status = StatusSnapshot{Drift: DriftCounts{InSync: 1}}
	settle(t, m, client, m.Update(SyncFinished{Outcome: SyncOutcome{ExitCode: 1, Changed: 2}}))
	view = m.View()
	expect(t, !view.Syncing && view.Headline.Key == text.StatusInSync, "finished=%+v", view)
	got := render(view.Notices[0])
	expect(t, got == "Last sync from here: finished with warnings, 2 changed", "notice=%q", got)
	checkText(t, statusTexts(view)...)
}

func TestStatusIgnoresLateSyncMessages(t *testing.T) {
	t.Parallel()
	client := &fakeClient{}
	m := readyStatus(t, client)
	expect(t, m.Update(SyncFinished{}) == nil, "finish without a sync reloaded")
	m.Update(SyncProgressed{Phase: "plan"})
	expect(t, m.syncPhase == "", "progress without a sync applied")
	m.Update(SyncNow{})
	settle(t, m, client, m.Update(SyncFinished{Err: &Error{Kind: ErrConflict}}))
	notices := m.View().Notices
	expect(t, notices[len(notices)-1].Key == text.ErrorConflict, "sync failure not shown: %+v", notices)
}

func TestStatusEvents(t *testing.T) {
	t.Parallel()
	m := readyStatus(t, &fakeClient{status: StatusSnapshot{Daemon: true}})
	only(t, m.Update(EventReceived{Event: Event{Kind: EventConnected}}))
	expect(t, m.View().Rows[5].Value.Key == text.StatusDaemonLive, "connection row=%+v", m.View().Rows[5])
	for _, kind := range []EventKind{EventDriftDetected, EventSyncFinished} {
		only(t, m.Update(EventReceived{Event: Event{Kind: kind}}))
	}
	expect(t, m.Update(EventReceived{Event: Event{Kind: EventUpdateAvailable}}) == nil, "update hint reloaded")
	expect(t, m.View().Notices[0].Key == text.StatusUpdate, "update notice missing")
	m.Update(SubscriptionEnded{Err: errBoom})
	expect(t, m.View().Rows[5].Value.Key == text.StatusDaemonQuiet, "disconnected row=%+v", m.View().Rows[5])
	m.snapshot.Daemon = false
	expect(t, m.View().Rows[5].Value.Key == text.StatusLocal, "local row=%+v", m.View().Rows[5])
	checkText(t, statusTexts(m.View())...)
}

// A foreign sync is cleared only by its finish hint or by a load issued after
// it started: a slow, older load must not erase the newer "sync running" state.
func TestStatusForeignSyncSurvivesOlderLoads(t *testing.T) {
	t.Parallel()
	m := readyStatus(t, &fakeClient{})
	before := loadSeq(t, m.Update(Refresh{}))
	m.Update(EventReceived{Event: Event{Kind: EventSyncStarted}})
	m.Update(StatusLoaded{Seq: before})
	view := m.View()
	expect(t, view.Headline.Key == text.StatusOtherSync, "older load cleared foreign sync: %+v", view.Headline)
	expect(t, !action(t, view.Actions, SyncNow{}).Enabled, "sync offered during a foreign sync")
	expect(t, m.Update(SyncNow{}) == nil, "sync started during a foreign sync")
	m.Update(StatusLoaded{Seq: loadSeq(t, m.Update(Refresh{}))})
	expect(t, m.View().Headline.Key == text.StatusInSync, "newer successful load did not clear the foreign sync")
	m.Update(EventReceived{Event: Event{Kind: EventSyncStarted}})
	m.Update(EventReceived{Event: Event{Kind: EventSyncFinished}})
	expect(t, !m.otherSync, "finish hint did not clear the foreign sync")
	m.Update(SyncNow{})
	m.Update(EventReceived{Event: Event{Kind: EventSyncStarted}})
	expect(t, !m.otherSync, "own sync's start hint reported as foreign")
}

func TestStatusRows(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 24, 8, 30, 0, 0, time.FixedZone("CET", 3600))
	m := readyStatus(t, &fakeClient{status: StatusSnapshot{
		Repository: "/repo", Daemon: true, Warnings: []string{"a: unsupported"},
		Git:      GitState{Branch: "main", Upstream: "origin/main", Ahead: 1, Behind: 2, Dirty: true},
		LastSync: &LastSync{At: at, ExitCode: 0, Changed: 3},
		Drift:    DriftCounts{InSync: 4, UpdateNeeded: 1},
	}})
	view := m.View()
	want := []string{
		"Repository: /repo",
		"Branch: main → origin/main (1 ahead, 2 behind)",
		"Working copy: Uncommitted changes",
		"Last sync: 2026-10-24T07:30:00Z: succeeded, 3 changed",
		"Targets: 4 in sync, 1 to update, 0 changed locally, 0 conflicts, 0 obstacles",
		"Connection: Daemon, live updates unavailable",
	}
	expect(t, len(view.Rows) == len(want), "rows=%+v", view.Rows)
	for i, row := range view.Rows {
		got := render(row.Label) + ": " + render(row.Value)
		expect(t, got == want[i], "row %d=%q want %q", i, got, want[i])
	}
	expect(t, render(view.Warnings[0]) == "a: unsupported", "warnings=%+v", view.Warnings)
	m.snapshot.Git, m.snapshot.LastSync = GitState{Branch: "main", UpstreamGone: true, Upstream: "origin/main"}, nil
	rows := m.View().Rows
	expect(t, render(rows[1].Value) == "main (no upstream)", "branch=%q", render(rows[1].Value))
	expect(t, render(rows[2].Value) == "Clean", "copy=%q", render(rows[2].Value))
	expect(t, rows[3].Value.Key == text.StatusNeverSynced, "last=%+v", rows[3])
}
