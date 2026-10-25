package model

import "time"

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
	retry    time.Duration
}

// NewApp builds all models; logCapacity bounds the activity log.
func NewApp(logCapacity int) *App {
	return &App{Status: NewStatus(), Drift: NewDrift(), Settings: NewSettings(), Log: NewLog(logCapacity)}
}

// Init loads every snapshot and starts the subscription.
func (a *App) Init() []Cmd {
	cmds := a.Status.Init()
	cmds = append(cmds, a.Drift.Init()...)
	cmds = append(cmds, a.Settings.Init()...)
	return append(cmds, Subscribe{})
}

// Update routes msg to every model. A lost subscription is retried with
// exponential backoff unless it ended by cancellation (renderer shutdown).
func (a *App) Update(msg Msg) []Cmd {
	cmds := a.Status.Update(msg)
	cmds = append(cmds, a.Drift.Update(msg)...)
	cmds = append(cmds, a.Settings.Update(msg)...)
	cmds = append(cmds, a.Log.Update(msg)...)
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
