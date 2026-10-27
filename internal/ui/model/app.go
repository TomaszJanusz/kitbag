package model

import (
	"time"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
)

// Resubscription backoff: the first retry follows quickly, repeated failures
// (for example while no daemon runs) back off to a ceiling.
const (
	RetryInitial = time.Second
	RetryMaximum = 30 * time.Second
	retryFactor  = 2
)

// App composes the models a renderer shows and owns the event subscription.
// Every Msg reaches every model; Cmds from all of them are returned together.
type App struct {
	Status   *Status
	Drift    *Drift
	Settings *Settings
	Log      *Log
	// Agents lists agent instances; AgentForm adds, edits or removes one;
	// Upgrade converts kitbag.toml to schema 2 from the form's hosts step.
	Agents    *Agents
	AgentForm *AgentForm
	Upgrade   *ManifestUpgrade
	retry     time.Duration
}

// NewApp builds all models; logCapacity bounds the activity log and clock
// decides the zone and relative hints of every time shown.
func NewApp(logCapacity int, clock timefmt.Clock) *App {
	status, log := NewStatus(), NewLog(logCapacity)
	status.clock, log.clock = clock, clock
	return &App{Status: status, Drift: NewDrift(), Settings: NewSettings(), Log: log, Agents: NewAgents(), AgentForm: NewAgentForm(), Upgrade: NewManifestUpgrade()}
}

// Init loads every snapshot and starts the subscription.
func (a *App) Init() []Cmd {
	cmds := a.Status.Init()
	cmds = append(cmds, a.Drift.Init()...)
	cmds = append(cmds, a.Settings.Init()...)
	cmds = append(cmds, a.Agents.Init()...)
	return append(cmds, Subscribe{})
}

// Update routes msg to every model. A lost subscription is retried with
// exponential backoff unless it ended by cancellation (renderer shutdown).
func (a *App) Update(msg Msg) []Cmd {
	cmds := a.Status.Update(msg)
	cmds = append(cmds, a.Drift.Update(msg)...)
	cmds = append(cmds, a.Settings.Update(msg)...)
	cmds = append(cmds, a.Log.Update(msg)...)
	cmds = append(cmds, a.Agents.Update(msg)...)
	cmds = append(cmds, a.AgentForm.Update(msg)...)
	cmds = append(cmds, a.Upgrade.Update(msg)...)
	switch msg := msg.(type) {
	case EventReceived:
		if msg.Event.Kind == EventConnected {
			a.retry = 0
		}
	case SubscriptionEnded:
		if problem := Classify(msg.Err); problem != nil && problem.Kind == ErrCanceled {
			return cmds
		}
		a.retry = min(max(a.retry*retryFactor, RetryInitial), RetryMaximum)
		cmds = append(cmds, Subscribe{Delay: a.retry})
	}
	return cmds
}
