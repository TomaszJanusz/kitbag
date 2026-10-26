package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

type unknownCmd struct{}

func (unknownCmd) isCmd() {}

func TestRunnerLoads(t *testing.T) {
	t.Parallel()
	client := &fakeClient{status: StatusSnapshot{Repository: "/r"}, drift: driftSample(), settings: storedSettings()}
	status := as[StatusLoaded](t, only(t, execute(t, client, LoadStatus{Seq: 7})))
	expect(t, status.Seq == 7 && status.Snapshot.Repository == "/r", "status=%+v", status)
	drift := as[DriftLoaded](t, only(t, execute(t, client, LoadDrift{Seq: 2})))
	expect(t, drift.Seq == 2 && len(drift.Snapshot.Items) == 3, "drift=%+v", drift)
	loaded := as[SettingsLoaded](t, only(t, execute(t, client, LoadSettings{Seq: 3})))
	expect(t, loaded.Seq == 3 && len(loaded.Values) == len(storedSettings()), "settings=%+v", loaded)
	expect(t, len(execute(t, client, unknownCmd{})) == 0, "unknown command emitted")
	expect(t, (Runner{}).now().IsZero(), "nil clock")
}

func TestRunnerSyncAndWrites(t *testing.T) {
	t.Parallel()
	client := &fakeClient{phases: []string{"plan"}, outcome: SyncOutcome{ExitCode: 1}}
	got := execute(t, client, RunSync{})
	expect(t, len(got) == 2, "sync=%v", got)
	expect(t, got[0] == (SyncProgressed{At: fixedNow(), Phase: "plan"}), "progress=%+v", got[0])
	expect(t, as[SyncFinished](t, got[1]).Outcome.ExitCode == 1, "finished=%+v", got[1])
	execute(t, client, WriteSetting{Key: "ui.theme", Value: "dark"})
	written := as[SettingWritten](t, only(t, execute(t, client, WriteSetting{Key: "ui.theme", Unset: true})))
	expect(t, written.Key == "ui.theme" && written.At.Equal(fixedNow()), "written=%+v", written)
	expect(t, len(client.writes) == 2 && !client.writes[0].Unset && client.writes[1].Unset, "writes=%v", client.writes)
}

func TestRunnerSubscribe(t *testing.T) {
	t.Parallel()
	client := &fakeClient{events: []Event{{Kind: EventDriftDetected}}}
	got := execute(t, client, Subscribe{Delay: time.Millisecond})
	expect(t, len(got) == 3, "subscribe=%v", got)
	expect(t, as[EventReceived](t, got[0]).Event.Kind == EventConnected, "first=%+v", got[0])
	expect(t, as[SubscriptionEnded](t, got[2]).Err == nil, "end=%+v", got[2])
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var ended []Msg
	Runner{Client: client}.Run(ctx, Subscribe{Delay: time.Hour}, func(msg Msg) { ended = append(ended, msg) })
	expect(t, errors.Is(as[SubscriptionEnded](t, only(t, ended)).Err, context.Canceled), "ended=%v", ended)
	expect(t, client.subscribes == 1, "a canceled wait still subscribed")
}

func TestClassify(t *testing.T) {
	t.Parallel()
	classified := &Error{Kind: ErrBusy, Detail: "locked"}
	expect(t, Classify(nil) == nil, "nil classified")
	expect(t, Classify(classified) == classified, "classified error rewrapped")
	expect(t, Classify(context.Canceled).Kind == ErrCanceled, "cancellation")
	other := Classify(errBoom)
	expect(t, other.Kind == ErrOperation && other.ExitCode == 1, "other=%+v", other)
	expect(t, other.Error() == "boom" && errors.Is(other, errBoom), "other lost its cause: %+v", other)
	for _, kind := range []ErrorKind{ErrOperation, ErrUnavailable, ErrBusy, ErrInvalid, ErrConflict, ErrToolTooOld, ErrUnauthorized, ErrIncompatible, ErrUnsupportedOS, ErrCanceled, "future"} {
		checkText(t, ProblemText(&Error{Kind: kind}))
	}
	for code := -1; code <= 6; code++ {
		checkText(t, ResultText(code))
	}
	expect(t, render(ResultText(9)) == "finished with code 9", "unknown code")
	expect(t, render(PhaseText("teleport")) == "working", "unknown phase")
}

func TestRunnerAgentCommands(t *testing.T) {
	t.Parallel()
	client := &fakeClient{agents: agentFixture(), preview: AgentPreview{Revision: "r"}, applied: AgentApplied{ExitCode: 1}, applyErr: errBoom}
	loaded := as[AgentsLoaded](t, only(t, execute(t, client, LoadAgents{Seq: 3, Probe: true})))
	expect(t, loaded.Seq == 3 && len(loaded.Snapshot.Instances) == 3 && client.probes[0], "load=%+v", loaded)
	change := AgentChange{Op: AgentAdd, Name: "x"}
	previewed := as[AgentPreviewed](t, only(t, execute(t, client, PreviewAgent{Seq: 4, Change: change})))
	expect(t, previewed.Seq == 4 && previewed.Preview.Revision == "r" && client.changes[0] == change, "preview=%+v", previewed)
	applied := as[AgentChangeApplied](t, only(t, execute(t, client, ApplyAgent{Change: change, Revision: "r"})))
	expect(t, errors.Is(applied.Err, errBoom) && applied.Applied.ExitCode == 1 && client.revisions[0] == "r" && applied.At.Equal(fixedNow()), "apply=%+v", applied)
}
