package model

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
)

func TestAppInitAndRouting(t *testing.T) {
	t.Parallel()
	app := NewApp(8, timefmt.Clock{})
	cmds := app.Init()
	want := []Cmd{LoadStatus{Seq: 1}, LoadDrift{Seq: 1}, LoadSettings{Seq: 1}, LoadAgents{Seq: 1}, Subscribe{}}
	if !slices.Equal(cmds, want) {
		t.Fatalf("init=%v", cmds)
	}
	if app.Log.Capacity() != 8 {
		t.Fatal("log capacity not applied")
	}
	if cmds = app.Update(Refresh{}); len(cmds) != 4 {
		t.Fatalf("refresh=%v", cmds)
	}
	// A connection reloads status, drift and agents: hints may have been missed.
	if cmds = app.Update(EventReceived{Event: Event{Kind: EventConnected}}); len(cmds) != 3 || len(app.Log.Entries()) != 1 {
		t.Fatalf("connected=%v", cmds)
	}
}

func TestAppResubscribesWithBackoff(t *testing.T) {
	t.Parallel()
	app := NewApp(DefaultLogCapacity, timefmt.Clock{})
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, RetryMaximum, RetryMaximum}
	for i := range want {
		delay := as[Subscribe](t, only(t, app.Update(SubscriptionEnded{Err: &Error{Kind: ErrUnavailable}}))).Delay
		expect(t, delay == want[i], "retry %d after %v want %v", i, delay, want[i])
	}
	app.Update(EventReceived{Event: Event{Kind: EventConnected}})
	if cmds := app.Update(SubscriptionEnded{}); cmds[len(cmds)-1] != (Subscribe{Delay: RetryInitial}) {
		t.Fatalf("backoff not reset after connecting: %v", cmds)
	}
	if cmds := app.Update(SubscriptionEnded{Err: context.Canceled}); len(cmds) != 0 {
		t.Fatalf("shutdown resubscribed: %v", cmds)
	}
}

func TestAppDrivenThroughRunner(t *testing.T) {
	t.Parallel()
	client := &fakeClient{
		status:   StatusSnapshot{Daemon: true, Drift: DriftCounts{UpdateNeeded: 1}},
		drift:    DriftSnapshot{Items: []DriftItem{{Path: "/a", Classification: UpdateNeeded}}},
		settings: storedSettings(),
		phases:   []string{"started", "pull", "plan", "apply", "includes", "finished"},
		outcome:  SyncOutcome{Changed: 1},
		events:   []Event{{Kind: EventUpdateAvailable}},
	}
	app := NewApp(DefaultLogCapacity, timefmt.Clock{})
	settle(t, app, client, app.Init())
	for _, msg := range execute(t, client, Subscribe{}) {
		settle(t, app, client, app.Update(msg))
	}
	if app.Status.View().Phase != Ready || app.Drift.View().Phase != Ready || app.Settings.View().Phase != Ready || !app.Status.update {
		t.Fatal("app did not reach ready")
	}
	client.status.Drift = DriftCounts{InSync: 1}
	client.drift = DriftSnapshot{}
	settle(t, app, client, app.Update(SyncNow{}))
	if client.syncs != 1 || len(app.Drift.View().Rows) != 0 || app.Status.View().Notices[1].Args[1].Text != "1" {
		t.Fatalf("sync did not refresh both snapshots: syncs=%d", client.syncs)
	}
	// connected, update, disconnected (ctx not canceled: nil end), six phases, sync result.
	if got := len(app.Log.Entries()); got != 10 {
		t.Fatalf("log entries=%d", got)
	}
}

func TestAppRoutesAgentForm(t *testing.T) {
	t.Parallel()
	app := NewApp(DefaultLogCapacity, timefmt.Clock{})
	cmd := only(t, app.Update(OpenAgentForm{Op: AgentRemove, Target: "claude", Name: "second"}))
	expect(t, as[PreviewAgent](t, cmd).Change.Name == "second" && app.AgentForm.Open(), "form not routed")
	expect(t, len(app.Update(AgentChangeApplied{})) == 1, "agents reload after a change")
}
