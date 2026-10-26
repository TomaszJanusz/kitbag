package model

import (
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestLogIsBoundedOldestFirst(t *testing.T) {
	t.Parallel()
	const capacity = 4
	m := NewLog(capacity)
	base := fixedNow()
	for i := range capacity + 3 {
		m.Update(SyncProgressed{At: base.Add(time.Duration(i) * time.Second), Phase: "plan"})
	}
	entries := m.Entries()
	if len(entries) != capacity || m.Capacity() != capacity || m.Dropped() != 3 {
		t.Fatalf("len=%d cap=%d dropped=%d", len(entries), m.Capacity(), m.Dropped())
	}
	for i, entry := range entries {
		if want := base.Add(time.Duration(i+3) * time.Second); !entry.At.Equal(want) {
			t.Fatalf("entry %d at %v want %v", i, entry.At, want)
		}
	}
	entries[0].At = time.Time{}
	if m.Entries()[0].At.IsZero() {
		t.Fatal("Entries exposed internal storage")
	}
	if NewLog(0).Capacity() != DefaultLogCapacity || NewLog(-1).Capacity() != DefaultLogCapacity {
		t.Fatal("default capacity not applied")
	}
}

func TestLogRecordsActivity(t *testing.T) {
	t.Parallel()
	m := NewLog(DefaultLogCapacity)
	for _, msg := range []Msg{
		SubscriptionEnded{Err: errBoom},
		EventReceived{Event: Event{Kind: EventConnected}},
		EventReceived{Event: Event{Kind: EventSyncStarted}},
		EventReceived{Event: Event{Kind: EventSyncFinished, ExitCode: 3, Changed: 1}},
		EventReceived{Event: Event{Kind: EventDriftDetected}},
		EventReceived{Event: Event{Kind: EventUpdateAvailable}},
		EventReceived{Event: Event{Kind: "future.hint"}},
		SubscriptionEnded{Err: errBoom},
		SubscriptionEnded{Err: errBoom},
		SyncProgressed{Phase: "pull"},
		SyncFinished{Outcome: SyncOutcome{Changed: 2}},
		SyncFinished{Err: &Error{Kind: ErrBusy}},
		SettingWritten{Key: "ui.theme"},
		SettingWritten{Key: "ui.theme", Err: &Error{Kind: ErrInvalid}},
		Refresh{},
	} {
		if cmds := m.Update(msg); cmds != nil {
			t.Fatalf("log issued commands: %v", cmds)
		}
	}
	want := []struct {
		text  string
		level Severity
	}{
		{"Connected to the daemon", Info},
		{"A sync started", Info},
		{"A sync stopped on a conflict, 1 changed", Warning},
		{"The daemon detected drift", Warning},
		{"A newer kitbag release is available", Info},
		{"Unrecognized event future.hint", Info},
		{"Live updates disconnected", Warning},
		{"Sync: pulling the repository", Info},
		{"Sync succeeded, 2 changed", Info},
		{"Sync failed: Another kitbag operation is running; try again shortly", Failure},
		{"Saved ui.theme", Info},
		{"Could not save ui.theme: The configuration or request is invalid", Failure},
	}
	view := m.View()
	if len(view.Entries) != len(want) || view.Title.Key != text.LogTitle || view.Dropped != 0 {
		t.Fatalf("entries=%d %+v", len(view.Entries), view.Entries)
	}
	for i, entry := range view.Entries {
		checkText(t, entry.Text)
		if got := render(entry.Text); got != want[i].text || entry.Severity != want[i].level {
			t.Errorf("entry %d=%q/%d want %q/%d", i, got, entry.Severity, want[i].text, want[i].level)
		}
	}
}

func TestLogViewStampsEntriesInTheClockZone(t *testing.T) {
	t.Parallel()
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	m := NewLog(4)
	m.clock = timefmt.Clock{Location: warsaw}
	m.Update(EventReceived{At: time.Date(2026, 10, 26, 22, 59, 58, 0, time.UTC), Event: Event{Kind: EventDriftDetected}})
	if got := m.View().Entries[0].Time; got != "23:59:58" {
		t.Fatalf("Time=%q want 23:59:58", got)
	}
	if got := m.Entries()[0].Time; got != "" {
		t.Fatalf("Entries must stay raw, Time=%q", got)
	}
}
