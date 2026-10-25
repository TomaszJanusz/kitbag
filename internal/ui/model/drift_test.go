package model

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func driftTexts(v DriftView) []text.Message {
	messages := []text.Message{v.Title, v.Headline}
	messages = append(messages, v.Warnings...)
	if v.Problem != nil {
		messages = append(messages, *v.Problem)
	}
	for _, row := range v.Rows {
		messages = append(messages, row.Path, row.Classification)
	}
	if v.Detail != nil {
		messages = append(messages, v.Detail.Path, v.Detail.Classification, v.Detail.Diff)
		messages = append(messages, v.Detail.Actions...)
	}
	return messages
}

func driftSample() DriftSnapshot {
	return DriftSnapshot{Daemon: true, Warnings: []string{"adapter warning"}, Items: []DriftItem{
		{Path: "/a", Classification: UpdateNeeded, Actions: []Action{{Kind: ActionMaterialize, SourcePath: "/repo/a"}}},
		{Path: "/b", Classification: LocalDrift, Actions: []Action{{Kind: ActionBackup, BackupPath: "/b.bak"}, {Kind: ActionSkip, Warning: "edited"}}},
		{Path: "/c", Classification: Conflict},
	}}
}

func readyDrift(t *testing.T, snapshot DriftSnapshot) *Drift {
	t.Helper()
	m := NewDrift()
	settle(t, m, &fakeClient{drift: snapshot}, m.Init())
	return m
}

func renderAll(messages []text.Message) []string {
	result := make([]string, 0, len(messages))
	for _, m := range messages {
		result = append(result, render(m))
	}
	return result
}

func TestDriftLoadingFailureAndEmpty(t *testing.T) {
	t.Parallel()
	m := NewDrift()
	cmds := m.Init()
	view := m.View()
	expect(t, view.Phase == Loading && view.Headline.Key == text.DriftLoading, "loading=%+v", view)
	expect(t, view.Detail == nil, "detail before load")
	settle(t, m, &fakeClient{driftErr: &Error{Kind: ErrBusy}}, cmds)
	view = m.View()
	expect(t, view.Phase == Failed && view.Headline.Key == text.ErrorBusy, "failed=%+v", view)
	settle(t, m, &fakeClient{}, m.Update(Refresh{}))
	view = m.View()
	expect(t, view.Phase == Ready && view.Headline.Key == text.DriftEmpty, "empty=%+v", view)
	expect(t, view.Problem == nil && view.Detail == nil, "empty view kept stale state: %+v", view)
	checkText(t, driftTexts(view)...)
}

func TestDriftReadyKeepsListOnFailedRefresh(t *testing.T) {
	t.Parallel()
	m := readyDrift(t, driftSample())
	view := m.View()
	expect(t, render(view.Headline) == "3 targets differ", "headline=%q", render(view.Headline))
	expect(t, view.Rows[0].Selected && render(view.Detail.Path) == "/a", "first item not selected: %+v", view)
	settle(t, m, &fakeClient{driftErr: errBoom}, m.Update(Refresh{}))
	view = m.View()
	expect(t, view.Phase == Ready && len(view.Rows) == 3, "a failed refresh dropped the list: %+v", view)
	expect(t, view.Problem != nil && view.Problem.Key == text.ErrorOperation, "problem=%+v", view.Problem)
	checkText(t, driftTexts(view)...)
}

func TestDriftDetail(t *testing.T) {
	t.Parallel()
	m := readyDrift(t, driftSample())
	m.Update(MoveDrift{Delta: 1})
	detail := m.View().Detail
	got := renderAll(detail.Actions)
	want := []string{"Back up the current file", "Backup: /b.bak", "Skip and leave unchanged", "Warning: edited"}
	expect(t, render(detail.Classification) == "Changed locally", "classification=%q", render(detail.Classification))
	expect(t, detail.Diff.Key == text.DriftNoDiff, "diff=%+v", detail.Diff)
	expect(t, len(got) == len(want), "detail=%q", got)
	for i := range want {
		expect(t, got[i] == want[i], "line %d=%q want %q", i, got[i], want[i])
	}
	m.Update(MoveDrift{Delta: 10})
	expect(t, render(m.View().Detail.Actions[0]) == "No action planned", "not clamped to last: %+v", m.View().Detail)
	m.Update(SelectDrift{Index: -4})
	expect(t, m.selected == "/a", "not clamped to first: %q", m.selected)
	expect(t, render(m.View().Detail.Actions[1]) == "Source: /repo/a", "source line missing")
	checkText(t, driftTexts(m.View())...)
}

func TestDriftSelectionFollowsPath(t *testing.T) {
	t.Parallel()
	m := readyDrift(t, driftSample())
	m.Update(SelectDrift{Index: 2})
	shifted := driftSample()
	shifted.Items = append([]DriftItem{{Path: "/0", Classification: UnmanagedObstacle}}, shifted.Items...)
	settle(t, m, &fakeClient{drift: shifted}, m.Update(Refresh{}))
	expect(t, m.selected == "/c" && m.View().Rows[3].Selected, "selection moved: %q", m.selected)
	settle(t, m, &fakeClient{drift: DriftSnapshot{Items: shifted.Items[:2]}}, m.Update(Refresh{}))
	expect(t, m.selected == "/0", "missing selection did not fall back to the first item: %q", m.selected)
	settle(t, m, &fakeClient{}, m.Update(Refresh{}))
	m.Update(MoveDrift{Delta: 1})
	expect(t, m.selected == "" && m.View().Detail == nil, "empty list kept a selection")
}

func TestDriftUnknownVocabularyStaysVisible(t *testing.T) {
	t.Parallel()
	m := readyDrift(t, DriftSnapshot{Items: []DriftItem{{Path: "/x", Classification: "future", Actions: []Action{{Kind: "teleport"}}}}})
	view := m.View()
	expect(t, render(view.Rows[0].Classification) == "future", "classification=%+v", view.Rows[0])
	expect(t, render(view.Detail.Actions[0]) == "Planned action: teleport", "action=%+v", view.Detail.Actions)
	checkText(t, driftTexts(view)...)
}

func TestDriftRefreshTriggersAndStaleResults(t *testing.T) {
	t.Parallel()
	m := NewDrift()
	first := as[LoadDrift](t, only(t, m.Init())).Seq
	for _, msg := range []Msg{SyncFinished{}, EventReceived{Event: Event{Kind: EventConnected}}, EventReceived{Event: Event{Kind: EventSyncFinished}}, EventReceived{Event: Event{Kind: EventDriftDetected}}} {
		only(t, m.Update(msg))
	}
	for _, kind := range []EventKind{EventSyncStarted, EventUpdateAvailable} {
		expect(t, m.Update(EventReceived{Event: Event{Kind: kind}}) == nil, "%s reloaded", kind)
	}
	m.Update(DriftLoaded{Seq: m.issued, Snapshot: driftSample()})
	m.Update(DriftLoaded{Seq: first, Snapshot: DriftSnapshot{}})
	expect(t, len(m.snapshot.Items) == 3, "an older plan overwrote a newer one")
}
