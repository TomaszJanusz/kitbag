package model

import (
	"context"
	"time"
)

// Runner executes Cmds against a Client. Run blocks until the command ends;
// a renderer typically calls it from its own worker and feeds emitted Msgs
// back into its single update loop. Now stamps time-bearing results.
type Runner struct {
	Client Client
	Now    func() time.Time
}

// Run executes cmd and emits zero or more progress Msgs followed by exactly
// one result Msg. Unknown commands emit nothing.
func (r Runner) Run(ctx context.Context, cmd Cmd, emit func(Msg)) {
	switch c := cmd.(type) {
	case LoadStatus:
		snapshot, err := r.Client.Status(ctx)
		emit(StatusLoaded{Seq: c.Seq, Snapshot: snapshot, Err: err})
	case LoadDrift:
		snapshot, err := r.Client.Drift(ctx)
		emit(DriftLoaded{Seq: c.Seq, Snapshot: snapshot, Err: err})
	case LoadSettings:
		values, err := r.Client.Settings(ctx)
		emit(SettingsLoaded{Seq: c.Seq, Values: values, Err: err})
	case WriteSetting:
		emit(SettingWritten{At: r.now(), Key: c.Key, Err: r.write(ctx, c)})
	case RunServiceRefresh:
		emit(ServiceRefreshed{Err: r.Client.RefreshService(ctx)})
	case RunSync:
		outcome, err := r.Client.Sync(ctx, func(phase string) { emit(SyncProgressed{At: r.now(), Phase: phase}) })
		emit(SyncFinished{At: r.now(), Outcome: outcome, Err: err})
	case Subscribe:
		emit(SubscriptionEnded{Err: r.subscribe(ctx, c, emit), At: r.now()})
	case LoadAgents, PreviewAgent, ApplyAgent, PreviewManifestUpgrade, ApplyManifestUpgrade:
		r.runAgents(ctx, cmd, emit)
	}
}

// runAgents executes the agent commands.
func (r Runner) runAgents(ctx context.Context, cmd Cmd, emit func(Msg)) {
	switch c := cmd.(type) {
	case LoadAgents:
		snapshot, err := r.Client.Agents(ctx, c.Probe)
		emit(AgentsLoaded{Seq: c.Seq, Snapshot: snapshot, Err: err})
	case PreviewAgent:
		preview, err := r.Client.PreviewAgentChange(ctx, c.Change)
		emit(AgentPreviewed{Seq: c.Seq, Preview: preview, Err: err})
	case ApplyAgent:
		applied, err := r.Client.ApplyAgentChange(ctx, c.Change, c.Revision)
		emit(AgentChangeApplied{At: r.now(), Applied: applied, Err: err})
	case PreviewManifestUpgrade:
		preview, err := r.Client.PreviewManifestUpgrade(ctx)
		emit(ManifestUpgradePreviewed{Seq: c.Seq, Preview: preview, Err: err})
	case ApplyManifestUpgrade:
		applied, err := r.Client.ApplyManifestUpgrade(ctx, c.Revision)
		emit(ManifestUpgradeDone{Applied: applied, Err: err})
	}
}

func (r Runner) write(ctx context.Context, c WriteSetting) error {
	if c.Unset {
		return r.Client.UnsetSetting(ctx, c.Key)
	}
	return r.Client.SetSetting(ctx, c.Key, c.Value)
}

func (r Runner) subscribe(ctx context.Context, c Subscribe, emit func(Msg)) error {
	if c.Delay > 0 {
		timer := time.NewTimer(c.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return r.Client.Subscribe(ctx, func(event Event) { emit(EventReceived{At: r.now(), Event: event}) })
}

func (r Runner) now() time.Time {
	if r.Now == nil {
		return time.Time{}
	}
	return r.Now()
}
