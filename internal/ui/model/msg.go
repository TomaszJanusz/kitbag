package model

import "time"

// Msg is a model input: a user intent or the result of a Cmd.
type Msg interface{ isMsg() }

// Cmd is work for the renderer's runtime; execute it with Runner.Run.
type Cmd interface{ isCmd() }

// User intents.
type (
	// Refresh reloads status, drift and settings.
	Refresh struct{}
	// SyncNow runs one sync unless one is already running from here.
	SyncNow struct{}
	// SelectDrift selects the drift item at Index.
	SelectDrift struct{ Index int }
	// MoveDrift moves the drift selection by Delta, clamped to the list.
	MoveDrift struct{ Delta int }
	// ToggleAdvancedSettings expands or collapses less common settings.
	ToggleAdvancedSettings struct{}
	// BeginSetting chooses the appropriate input for the current draft.
	BeginSetting struct{ Key string }
	// StepSetting cycles a picker by Delta.
	StepSetting struct {
		Key   string
		Delta int
	}
	// ChooseSetting selects the option at Index, including custom input.
	ChooseSetting struct {
		Key   string
		Index int
	}
	// EditSetting replaces a field's draft and validates it immediately.
	EditSetting struct{ Key, Value string }
	// SaveSetting persists a valid, changed draft.
	SaveSetting struct{ Key string }
	// UnsetSetting removes a value stored in config.toml.
	UnsetSetting struct{ Key string }
	// RevertSetting discards a draft.
	RevertSetting struct{ Key string }
)

// Command results. Seq echoes the request so stale results are discarded.
type (
	// StatusLoaded carries a status.get result. A failed call may still carry a
	// snapshot, such as the in-process tool-too-old report.
	StatusLoaded struct {
		Err      error
		Snapshot StatusSnapshot
		Seq      uint64
	}
	// DriftLoaded carries a plan.get projection.
	DriftLoaded struct {
		Err      error
		Snapshot DriftSnapshot
		Seq      uint64
	}
	// SettingsLoaded carries effective settings.
	SettingsLoaded struct {
		Err    error
		Values []SettingValue
		Seq    uint64
	}
	// SettingWritten reports a set or unset.
	SettingWritten struct {
		At  time.Time
		Err error
		Key string
	}
	// SyncProgressed reports a coarse phase of this renderer's sync.
	SyncProgressed struct {
		At    time.Time
		Phase string
	}
	// SyncFinished ends this renderer's sync.
	SyncFinished struct {
		At      time.Time
		Err     error
		Outcome SyncOutcome
	}
	// EventReceived carries a subscription hint.
	EventReceived struct {
		At    time.Time
		Event Event
	}
	// SubscriptionEnded reports that the event stream stopped.
	SubscriptionEnded struct {
		At  time.Time
		Err error
	}
)

// Commands.
type (
	// LoadStatus calls Client.Status.
	LoadStatus struct{ Seq uint64 }
	// LoadDrift calls Client.Drift.
	LoadDrift struct{ Seq uint64 }
	// LoadSettings calls Client.Settings.
	LoadSettings struct{ Seq uint64 }
	// WriteSetting calls SetSetting, or UnsetSetting when Unset is true.
	WriteSetting struct {
		Key, Value string
		Unset      bool
	}
	// RunSync calls Client.Sync.
	RunSync struct{}
	// Subscribe waits Delay, then calls Client.Subscribe until it ends.
	Subscribe struct{ Delay time.Duration }
)

func (Refresh) isMsg()           {}
func (SyncNow) isMsg()           {}
func (SelectDrift) isMsg()       {}
func (MoveDrift) isMsg()         {}
func (EditSetting) isMsg()       {}
func (SaveSetting) isMsg()       {}
func (UnsetSetting) isMsg()      {}
func (RevertSetting) isMsg()     {}
func (StatusLoaded) isMsg()      {}
func (DriftLoaded) isMsg()       {}
func (SettingsLoaded) isMsg()    {}
func (SettingWritten) isMsg()    {}
func (SyncProgressed) isMsg()    {}
func (SyncFinished) isMsg()      {}
func (EventReceived) isMsg()     {}
func (SubscriptionEnded) isMsg() {}

func (LoadStatus) isCmd()   {}
func (LoadDrift) isCmd()    {}
func (LoadSettings) isCmd() {}
func (WriteSetting) isCmd() {}
func (RunSync) isCmd()      {}
func (Subscribe) isCmd()    {}

func (ToggleAdvancedSettings) isMsg() {}
func (BeginSetting) isMsg()           {}
func (StepSetting) isMsg()            {}
func (ChooseSetting) isMsg()          {}

// RequestServiceRefresh opens confirmation for an installed stale/mismatched service.
type (
	RequestServiceRefresh struct{}
	// ConfirmServiceRefresh answers that confirmation; only Accept runs work.
	ConfirmServiceRefresh struct{ Accept bool }
	// ServiceRefreshed reports the local refresh result.
	ServiceRefreshed struct{ Err error }
	// RunServiceRefresh performs an explicitly confirmed repair.
	RunServiceRefresh struct{}
)

func (RequestServiceRefresh) isMsg() {}
func (ConfirmServiceRefresh) isMsg() {}
func (ServiceRefreshed) isMsg()      {}
func (RunServiceRefresh) isCmd()     {}
