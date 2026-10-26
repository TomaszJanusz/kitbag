// Package text holds every user-visible string of the shared view models in
// one catalog, ready for later translation. Models emit Messages; renderers
// display them through a Catalog and never hard-code user text.
package text

import (
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/settingskey"
)

// Key identifies one translatable template.
type Key string

// Message is a key with positional arguments, rendered later. Arguments are
// messages too, so nested phrases stay translatable; a Message without a Key
// is Plain data such as a path, number or core diagnostic, shown verbatim.
type Message struct {
	Key  Key
	Text string
	Args []Message
}

// New builds a translatable Message.
func New(key Key, args ...Message) Message { return Message{Key: key, Args: args} }

// Plain wraps untranslated data.
func Plain(value string) Message { return Message{Text: value} }

// Catalog maps keys to fmt templates whose only verb is %s (%% is a literal).
type Catalog map[Key]string

// Render formats a message; an unknown key renders as the key itself, so a
// missing translation stays visible instead of silently disappearing.
func (c Catalog) Render(m Message) string {
	if m.Key == "" {
		return m.Text
	}
	template, ok := c[m.Key]
	if !ok {
		return string(m.Key)
	}
	args := make([]any, len(m.Args))
	for i, arg := range m.Args {
		args[i] = c.Render(arg)
	}
	return fmt.Sprintf(template, args...)
}

type entry struct {
	key      Key
	template string
}

// settingPrefix namespaces the descriptions taken from the settings registry.
const settingPrefix = "setting.description."

// SettingDescription is the key of a registry setting's description.
func SettingDescription(path string) Key { return Key(settingPrefix + path) }

// Static keys.
const (
	ActionSyncNow Key = "action.sync_now"
	ActionRefresh Key = "action.refresh"
	ActionSave    Key = "action.save"
	ActionUnset   Key = "action.unset"
	ActionRevert  Key = "action.revert"

	StatusTitle         Key = "status.title"
	StatusLoading       Key = "status.loading"
	StatusInSync        Key = "status.in_sync"
	StatusUpdates       Key = "status.updates"
	StatusLocalDrift    Key = "status.local_drift"
	StatusConflicts     Key = "status.conflicts"
	StatusSyncing       Key = "status.syncing"
	StatusOtherSync     Key = "status.other_sync"
	StatusToolTooOld    Key = "status.tool_too_old"
	StatusSchemaTooNew  Key = "status.schema_too_new"
	StatusUpdate        Key = "status.update_available"
	StatusStale         Key = "status.stale"
	StatusRepository    Key = "status.repository"
	StatusBranch        Key = "status.branch"
	StatusBranchValue   Key = "status.branch_value"
	StatusNoUpstream    Key = "status.no_upstream"
	StatusWorkingCopy   Key = "status.working_copy"
	StatusClean         Key = "status.clean"
	StatusDirty         Key = "status.dirty"
	StatusLastSync      Key = "status.last_sync"
	StatusLastSyncValue Key = "status.last_sync_value"
	StatusNeverSynced   Key = "status.never_synced"
	StatusDrift         Key = "status.drift"
	StatusDriftValue    Key = "status.drift_value"
	StatusConnection    Key = "status.connection"
	StatusDaemonLive    Key = "status.daemon_live"
	StatusDaemonQuiet   Key = "status.daemon_quiet"
	StatusLocal         Key = "status.local"
	StatusSyncResult    Key = "status.sync_result"

	PhaseStarted  Key = "phase.started"
	PhasePull     Key = "phase.pull"
	PhasePlan     Key = "phase.plan"
	PhaseApply    Key = "phase.apply"
	PhaseIncludes Key = "phase.includes"
	PhaseFinished Key = "phase.finished"
	PhaseOther    Key = "phase.other"

	ResultOK            Key = "result.ok"
	ResultWarnings      Key = "result.warnings"
	ResultUsage         Key = "result.usage"
	ResultConflict      Key = "result.conflict"
	ResultToolTooOld    Key = "result.tool_too_old"
	ResultUnsupportedOS Key = "result.unsupported_os"
	ResultUnknown       Key = "result.unknown"

	ErrorOperation     Key = "error.operation"
	ErrorUnavailable   Key = "error.unavailable"
	ErrorBusy          Key = "error.busy"
	ErrorInvalid       Key = "error.invalid"
	ErrorConflict      Key = "error.conflict"
	ErrorToolTooOld    Key = "error.tool_too_old"
	ErrorUnauthorized  Key = "error.unauthorized"
	ErrorIncompatible  Key = "error.incompatible"
	ErrorUnsupportedOS Key = "error.unsupported_os"
	ErrorCanceled      Key = "error.canceled"

	ClassInSync    Key = "class.in_sync"
	ClassUpdate    Key = "class.update_needed"
	ClassDrift     Key = "class.local_drift"
	ClassConflict  Key = "class.conflict"
	ClassUnmanaged Key = "class.unmanaged_obstacle"

	OpRemove      Key = "op.remove"
	OpDiscard     Key = "op.discard"
	OpBackup      Key = "op.backup"
	OpMaterialize Key = "op.materialize"
	OpWrite       Key = "op.write"
	OpMerge       Key = "op.merge"
	OpUnmerge     Key = "op.unmerge"
	OpRestore     Key = "op.restore"
	OpRecord      Key = "op.record"
	OpRelease     Key = "op.release"
	OpSkip        Key = "op.skip"
	OpOther       Key = "op.other"
	OpWarning     Key = "op.warning"
	OpBackupPath  Key = "op.backup_path"
	OpSource      Key = "op.source"

	DriftTitle       Key = "drift.title"
	DriftLoading     Key = "drift.loading"
	DriftEmpty       Key = "drift.empty"
	DriftCount       Key = "drift.count"
	DriftNoDiff      Key = "drift.no_diff"
	DriftNoOperation Key = "drift.no_operation"

	SettingsTitle     Key = "settings.title"
	SettingsLoading   Key = "settings.loading"
	SettingsReady     Key = "settings.ready"
	SettingInvalid    Key = "setting.invalid"
	SettingRedacted   Key = "setting.redacted"
	SettingUnset      Key = "setting.unset"
	SettingDefault    Key = "setting.default"
	SettingSaving     Key = "setting.saving"
	SettingOverridden Key = "setting.overridden"
	SourceFlag        Key = "source.flag"
	SourceEnvironment Key = "source.environment"
	SourceFile        Key = "source.file"
	SourceDefault     Key = "source.default"
	SettingSaveFailed Key = "setting.save_failed"

	LogTitle         Key = "log.title"
	LogConnected     Key = "log.connected"
	LogDisconnected  Key = "log.disconnected"
	LogSyncStarted   Key = "log.sync_started"
	LogSyncFinished  Key = "log.sync_finished"
	LogDriftDetected Key = "log.drift_detected"
	LogUpdate        Key = "log.update_available"
	LogProgress      Key = "log.progress"
	LogOwnSync       Key = "log.own_sync"
	LogOwnSyncFailed Key = "log.own_sync_failed"
	LogSettingSaved  Key = "log.setting_saved"
	LogSettingFailed Key = "log.setting_failed"
	LogEventUnknown  Key = "log.event_unknown"
	LogEmpty         Key = "log.empty"

	TimeWithAgo     Key = "time.with_ago"
	TimeJustNow     Key = "time.just_now"
	TimeMinutesAgo  Key = "time.minutes_ago"
	TimeHoursAgo    Key = "time.hours_ago"
	TimeDaysAgo     Key = "time.days_ago"
	LogDropped      Key = "log.dropped"
	LogFollowing    Key = "log.following"
	LogPaused       Key = "log.paused"
	LogLevelInfo    Key = "log.level_info"
	LogLevelWarning Key = "log.level_warning"
	LogLevelFailure Key = "log.level_failure"

	// Terminal renderer chrome: tabs, help and layout notices.
	TerminalTooSmall Key = "terminal.too_small"
	TerminalFooter   Key = "terminal.footer"
	TerminalWarnings Key = "terminal.warnings"
	ControlHint      Key = "terminal.control_hint"
	ControlDisabled  Key = "terminal.control_disabled"
	DriftActions     Key = "terminal.drift_actions"
	DriftDetailHint  Key = "terminal.drift_detail_hint"
	DriftListHint    Key = "terminal.drift_list_hint"
	SettingValue     Key = "terminal.setting_value"
	SettingSource    Key = "terminal.setting_source"
	SettingChoices   Key = "terminal.setting_choices"
	SettingEditing   Key = "terminal.setting_editing"
	SettingListHint  Key = "terminal.setting_list_hint"
	HelpTitle        Key = "help.title"
	HelpTabs         Key = "help.tabs"
	HelpRefresh      Key = "help.refresh"
	HelpSync         Key = "help.sync"
	HelpMove         Key = "help.move"
	HelpDetail       Key = "help.detail"
	HelpEdit         Key = "help.edit"
	HelpSave         Key = "help.save"
	HelpChoices      Key = "help.choices"
	HelpFollow       Key = "help.follow"
	HelpMouse        Key = "help.mouse"
	HelpToggle       Key = "help.toggle"
	HelpQuit         Key = "help.quit"
)

// English returns the default catalog, including every registry setting
// description, so the settings registry stays the single source of that text.
func English() Catalog {
	catalog := make(Catalog)
	for _, section := range [][]entry{statusText(), outcomeText(), driftText(), formText(), terminalText()} {
		for _, e := range section {
			catalog[e.key] = e.template
		}
	}
	for _, key := range settingskey.Registry() {
		catalog[SettingDescription(key.Path)] = key.Description
	}
	return catalog
}

func statusText() []entry {
	return []entry{
		{ActionSyncNow, "Sync now"},
		{ActionRefresh, "Refresh"},
		{ActionSave, "Save"},
		{ActionUnset, "Remove from config.toml"},
		{ActionRevert, "Revert"},

		{StatusTitle, "Status"},
		{StatusLoading, "Loading status…"},
		{StatusInSync, "Everything is in sync"},
		{StatusUpdates, "%s targets need an update"},
		{StatusLocalDrift, "%s targets changed locally"},
		{StatusConflicts, "%s targets need attention"},
		{StatusSyncing, "Syncing: %s"},
		{StatusOtherSync, "Another sync is running"},
		{StatusToolTooOld, "This repository requires kitbag %s or newer; running %s"},
		{StatusSchemaTooNew, "This repository uses manifest schema %s; running kitbag %s, which reads schema %s"},
		{StatusUpdate, "A newer kitbag release is available"},
		{StatusStale, "Showing the last known status: %s"},
		{StatusRepository, "Repository"},
		{StatusBranch, "Branch"},
		{StatusBranchValue, "%s → %s (%s ahead, %s behind)"},
		{StatusNoUpstream, "%s (no upstream)"},
		{StatusWorkingCopy, "Working copy"},
		{StatusClean, "Clean"},
		{StatusDirty, "Uncommitted changes"},
		{StatusLastSync, "Last sync"},
		{StatusLastSyncValue, "%s: %s, %s changed"},
		{StatusNeverSynced, "Never synced on this machine"},
		{StatusDrift, "Targets"},
		{StatusDriftValue, "%s in sync, %s to update, %s changed locally, %s conflicts, %s obstacles"},
		{StatusConnection, "Connection"},
		{StatusDaemonLive, "Daemon, live updates"},
		{StatusDaemonQuiet, "Daemon, live updates unavailable"},
		{StatusLocal, "No daemon; reading this machine directly"},
		{StatusSyncResult, "Last sync from here: %s, %s changed"},
	}
}

func outcomeText() []entry {
	return []entry{
		{PhaseStarted, "starting"},
		{PhasePull, "pulling the repository"},
		{PhasePlan, "planning"},
		{PhaseApply, "applying changes"},
		{PhaseIncludes, "updating Git includes"},
		{PhaseFinished, "finishing"},
		{PhaseOther, "working"},

		{ResultOK, "succeeded"},
		{ResultWarnings, "finished with warnings"},
		{ResultUsage, "rejected the configuration"},
		{ResultConflict, "stopped on a conflict"},
		{ResultToolTooOld, "needs a newer kitbag"},
		{ResultUnsupportedOS, "is unsupported on this system"},
		{ResultUnknown, "finished with code %s"},

		{ErrorOperation, "The operation failed"},
		{ErrorUnavailable, "kitbag is not reachable"},
		{ErrorBusy, "Another kitbag operation is running; try again shortly"},
		{ErrorInvalid, "The configuration or request is invalid"},
		{ErrorConflict, "A conflict needs your attention"},
		{ErrorToolTooOld, "This repository requires a newer kitbag"},
		{ErrorUnauthorized, "The local API rejected the credentials"},
		{ErrorIncompatible, "The running daemon uses an incompatible API version"},
		{ErrorUnsupportedOS, "This operating system is not supported yet"},
		{ErrorCanceled, "Canceled"},
	}
}

func driftText() []entry {
	return []entry{
		{ClassInSync, "In sync"},
		{ClassUpdate, "Update needed"},
		{ClassDrift, "Changed locally"},
		{ClassConflict, "Conflict"},
		{ClassUnmanaged, "Unmanaged file in the way"},

		{OpRemove, "Remove the managed output"},
		{OpDiscard, "Discard reproducible output"},
		{OpBackup, "Back up the current file"},
		{OpMaterialize, "Create or replace from the repository"},
		{OpWrite, "Write generated content"},
		{OpMerge, "Merge settings"},
		{OpUnmerge, "Remove merged settings"},
		{OpRestore, "Restore the previous entry"},
		{OpRecord, "Record ownership only"},
		{OpRelease, "Stop managing"},
		{OpSkip, "Skip and leave unchanged"},
		{OpOther, "Planned action: %s"},
		{OpWarning, "Warning: %s"},
		{OpBackupPath, "Backup: %s"},
		{OpSource, "Source: %s"},

		{DriftTitle, "Drift"},
		{DriftLoading, "Inspecting targets…"},
		{DriftEmpty, "No drift: every target is in sync"},
		{DriftCount, "%s targets differ"},
		{DriftNoDiff, "Content diffs are not available from the local API yet"},
		{DriftNoOperation, "No action planned"},
	}
}

func formText() []entry {
	return []entry{
		{SettingsTitle, "Settings"},
		{SettingsLoading, "Loading settings…"},
		{SettingsReady, "Precedence: %s"},
		{SettingInvalid, "Allowed: %s"},
		{SettingRedacted, "[redacted]"},
		{SettingUnset, "unset"},
		{SettingDefault, "Default: %s"},
		{SettingSaving, "Saving…"},
		{SettingOverridden, "%s overrides config.toml"},
		{SourceFlag, "flag %s"},
		{SourceEnvironment, "environment %s"},
		{SourceFile, "config.toml"},
		{SourceDefault, "default"},
		{SettingSaveFailed, "Not saved: %s"},

		{LogTitle, "Activity"},
		{LogConnected, "Connected to the daemon"},
		{LogDisconnected, "Live updates disconnected"},
		{LogSyncStarted, "A sync started"},
		{LogSyncFinished, "A sync %s, %s changed"},
		{LogDriftDetected, "The daemon detected drift"},
		{LogUpdate, "A newer kitbag release is available"},
		{LogProgress, "Sync: %s"},
		{LogOwnSync, "Sync %s, %s changed"},
		{LogOwnSyncFailed, "Sync failed: %s"},
		{LogSettingSaved, "Saved %s"},
		{LogSettingFailed, "Could not save %s: %s"},
		{LogEventUnknown, "Unrecognized event %s"},
		{LogEmpty, "No activity yet"},
		{TimeWithAgo, "%s (%s)"},
		{TimeJustNow, "just now"},
		{TimeMinutesAgo, "%s min ago"},
		{TimeHoursAgo, "%s h ago"},
		{TimeDaysAgo, "%s d ago"},
		{LogDropped, "%s older entries dropped"},
		{LogFollowing, "Following new entries"},
		{LogPaused, "Scrolling; press f to follow new entries"},
		{LogLevelInfo, "info"},
		{LogLevelWarning, "warn"},
		{LogLevelFailure, "fail"},
	}
}

// terminalText is the chrome of the terminal renderer (kitbag tui).
func terminalText() []entry {
	return []entry{
		{TerminalTooSmall, "Enlarge the terminal to at least %s×%s (now %s×%s)"},
		{TerminalFooter, "? help · q quit · mouse: click, wheel"},
		{TerminalWarnings, "Warnings"},
		{ControlHint, "[%s] %s"},
		{ControlDisabled, "%s (unavailable)"},
		{DriftActions, "Planned actions"},
		{DriftDetailHint, "Esc: back to the list"},
		{DriftListHint, "↑/↓: select · Enter: details"},
		{SettingValue, "Value: %s"},
		{SettingSource, "Source: %s"},
		{SettingChoices, "Values: %s"},
		{SettingEditing, "Editing: Enter saves, Esc cancels"},
		{SettingListHint, "↑/↓: select · Enter: edit"},
		{HelpTitle, "Keyboard"},
		{HelpTabs, "Tab, Shift-Tab, 1–4: switch tabs"},
		{HelpRefresh, "r: reload status, drift and settings"},
		{HelpSync, "s: sync now"},
		{HelpMove, "↑/↓ or k/j: move the selection"},
		{HelpDetail, "Enter: open drift details; Esc: close them"},
		{HelpEdit, "Enter: edit a setting, Enter again saves; Esc cancels"},
		{HelpSave, "w: save a kept draft; u: remove the value from config.toml"},
		{HelpChoices, "←/→ while editing: cycle allowed values"},
		{HelpFollow, "f: follow new activity; ↑/↓, PgUp/PgDn: scroll"},
		{HelpMouse, "Mouse: click a tab or row, click a target again for details; wheel: scroll"},
		{HelpToggle, "?: show or hide this help"},
		{HelpQuit, "q or Ctrl-C: quit"},
	}
}
