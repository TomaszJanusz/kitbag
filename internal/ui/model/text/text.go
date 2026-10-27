// Package text holds every user-visible string of the shared view models in
// one catalog, ready for later translation. Models emit Messages; renderers
// display them through a Catalog and never hard-code user text.
package text

import (
	"fmt"
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

// settingPrefix namespaces the plain-language setting descriptions.
const settingPrefix = "setting.description."

// SettingDescription is the key of a registry setting's description.
func SettingDescription(path string) Key { return Key(settingPrefix + path) }

// Static keys.
const (
	FooterSave             Key = "footer.save"
	FooterCancel           Key = "footer.cancel"
	FooterHome             Key = "footer.home"
	FooterQuit             Key = "footer.quit"
	FooterHelp             Key = "footer.help"
	FooterBack             Key = "footer.back"
	ActionDetails          Key = "action.details"
	ActionFollow           Key = "action.follow"
	HomeBrand              Key = "home.brand"
	HomeCount              Key = "home.count"
	HomeAgentCount         Key = "home.agent_count"
	HomeVersion            Key = "home.version"
	HomeChecked            Key = "home.checked"
	HomeNextCheck          Key = "home.next_check"
	HomeRepository         Key = "home.repository"
	HomeFooter             Key = "home.footer"
	HomeUnavailable        Key = "home.unavailable"
	HomeCreatedBy          Key = "home.created_by"
	HomeMadeWith           Key = "home.made_with"
	HomeMadeWithPlain      Key = "home.made_with_plain"
	HomeSyncKey            Key = "home.sync_key"
	HomeAgentsKey          Key = "home.agents_key"
	HomeDriftKey           Key = "home.drift_key"
	HomeSettingsKey        Key = "home.settings_key"
	HomeActivityKey        Key = "home.activity_key"
	ScreenBreadcrumb       Key = "screen.breadcrumb"
	ScreenFooter           Key = "screen.footer"
	ActionRemoveShort      Key = "action.remove_short"
	SettingCompactHint     Key = "setting.compact_hint"
	DialogEscape           Key = "dialog.escape"
	DialogScroll           Key = "dialog.scroll"
	DialogOK               Key = "dialog.ok"
	DialogYes              Key = "dialog.yes"
	DialogNo               Key = "dialog.no"
	DialogBack             Key = "dialog.back"
	DialogNext             Key = "dialog.next"
	DialogFinish           Key = "dialog.finish"
	DialogStep             Key = "dialog.step"
	DialogHint             Key = "dialog.hint"
	HelpDialogs            Key = "help.dialogs"
	SettingHelp            Key = "setting.help"
	SettingUnsetTitle      Key = "setting.unset_title"
	SettingUnsetConfirm    Key = "setting.unset_confirm"
	SettingsAdvancedClosed Key = "settings.advanced_closed"
	SettingsAdvancedOpen   Key = "settings.advanced_open"
	SettingOn              Key = "setting.on"
	SettingOff             Key = "setting.off"
	SettingCustom          Key = "setting.custom"
	SettingOther           Key = "setting.other"
	SettingGitValue        Key = "setting.git_value"
	SettingGitUnavailable  Key = "setting.git_unavailable"
	SettingLegacyCLI       Key = "setting.legacy_cli"
	SettingPickerHint      Key = "setting.picker_hint"
	HelpAdvanced           Key = "help.advanced"
	ActionServiceRefresh   Key = "action.service_refresh"
	StatusVersions         Key = "status.versions"
	StatusVersionMismatch  Key = "status.version_mismatch"
	StatusServiceStale     Key = "status.service_stale"
	ServiceRefreshConfirm  Key = "service.refresh_confirm"
	ServiceRefreshing      Key = "service.refreshing"
	ServiceRefreshed       Key = "service.refreshed"
	ActionSyncNow          Key = "action.sync_now"
	ActionRefresh          Key = "action.refresh"
	ActionSave             Key = "action.save"
	ActionUnset            Key = "action.unset"
	ActionRevert           Key = "action.revert"

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
	StatusLastCheck     Key = "status.last_check"
	StatusNextCheck     Key = "status.next_check"
	StatusCheckUnknown  Key = "status.check_unknown"
	StatusCheckOK       Key = "status.check_ok"
	StatusCheckFailed   Key = "status.check_failed"
	StatusCheckValue    Key = "status.check_value"
	StatusCheckStopped  Key = "status.check_stopped"
	StatusCheckStuck    Key = "status.check_stuck"
	HelpCheckSync       Key = "help.check_sync"
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
	HelpAgents       Key = "help.agents"
	FooterAgents     Key = "terminal.footer_agents"
	DialogChoiceHint Key = "dialog.choice_hint"
	DialogScrollHint Key = "dialog.scroll_hint"
)

// Agents list, agent form and publication keys (ADR 12, v1 slice).
const (
	AgentsTitle           Key = "agents.title"
	AgentsLoading         Key = "agents.loading"
	AgentsEmpty           Key = "agents.empty"
	AgentsCount           Key = "agents.count"
	AgentsUndeclared      Key = "agents.undeclared"
	AgentsImplicit        Key = "agents.implicit"
	AgentsSourceDefault   Key = "agents.source.default"
	AgentsSourceEnv       Key = "agents.source.environment"
	AgentsSourceDeclared  Key = "agents.source.declared"
	AgentsSourceProbe     Key = "agents.source.probe"
	AgentsPresent         Key = "agents.presence.present"
	AgentsMissing         Key = "agents.presence.missing"
	AgentsNotDirectory    Key = "agents.presence.not_directory"
	AgentsSymlink         Key = "agents.presence.symlink"
	AgentsOutsideHome     Key = "agents.presence.outside_home"
	AgentsUnreadable      Key = "agents.presence.unreadable"
	AgentsScopeAll        Key = "agents.scope.all"
	AgentsScopeHosts      Key = "agents.scope.hosts"
	AgentsScopeExcept     Key = "agents.scope.except"
	AgentsActiveHere      Key = "agents.active_here"
	AgentsInactiveHere    Key = "agents.inactive_here"
	AgentsSettingsOn      Key = "agents.settings_on"
	AgentsSettingsOff     Key = "agents.settings_off"
	AgentsDetailDir       Key = "agents.detail.dir"
	AgentsDetailSource    Key = "agents.detail.source"
	AgentsDetailPresence  Key = "agents.detail.presence"
	AgentsDetailScope     Key = "agents.detail.scope"
	AgentsDetailState     Key = "agents.detail.state"
	AgentsDetailSettings  Key = "agents.detail.settings"
	ActionAddAgent        Key = "action.add_agent"
	ActionEditAgent       Key = "action.edit_agent"
	ActionRemoveAgent     Key = "action.remove_agent"
	ActionProbeAgents     Key = "action.probe_agents"
	WizardTitleAdd        Key = "wizard.title.add"
	WizardTitleEdit       Key = "wizard.title.edit"
	WizardTitleRemove     Key = "wizard.title.remove"
	WizardStepKind        Key = "wizard.step.kind"
	WizardStepName        Key = "wizard.step.name"
	WizardStepDir         Key = "wizard.step.dir"
	WizardStepSettings    Key = "wizard.step.settings"
	WizardStepPreview     Key = "wizard.step.preview"
	WizardStepConfirm     Key = "wizard.step.confirm"
	WizardStepResult      Key = "wizard.step.result"
	WizardExplainKind     Key = "wizard.explain.kind"
	WizardExplainName     Key = "wizard.explain.name"
	WizardExplainDir      Key = "wizard.explain.dir"
	WizardExplainSettings Key = "wizard.explain.settings"
	WizardExplainPreview  Key = "wizard.explain.preview"
	WizardExplainConfirm  Key = "wizard.explain.confirm"
	WizardExplainResult   Key = "wizard.explain.result"
	WizardSettingsDefault Key = "wizard.settings.default"
	WizardSettingsKeep    Key = "wizard.settings.keep"
	WizardSettingsOn      Key = "wizard.settings.on"
	WizardSettingsOff     Key = "wizard.settings.off"
	WizardHint            Key = "wizard.hint"
	WizardGeneric         Key = "wizard.generic"
	WizardBack            Key = "wizard.back"
	WizardNext            Key = "wizard.next"
	WizardCancel          Key = "wizard.cancel"
	WizardApply           Key = "wizard.apply"
	WizardClose           Key = "wizard.close"
	WizardDiscard         Key = "wizard.discard"
	WizardPreviewing      Key = "wizard.previewing"
	WizardApplying        Key = "wizard.applying"
	WizardCommit          Key = "wizard.commit"
	WizardDropped         Key = "wizard.dropped"
	AgentInvalidKind      Key = "agent.invalid.kind"
	AgentInvalidName      Key = "agent.invalid.name"
	AgentInvalidNameTaken Key = "agent.invalid.name_taken"
	AgentInvalidDir       Key = "agent.invalid.dir"
	AgentInvalidOutside   Key = "agent.invalid.outside_home"
	AgentInvalidDuplicate Key = "agent.invalid.duplicate"
	AgentWarnMissingDir   Key = "agent.warn.missing_dir"
	AgentErrorStale       Key = "agent.error.stale"
	AgentProblem          Key = "agent.problem"
	PublishRouteDirect    Key = "publish.route.direct"
	PublishRouteManual    Key = "publish.route.manual"
	AgentResultPublished  Key = "agent.result.published"
	AgentResultLocal      Key = "agent.result.not_published"
	AgentResultManual     Key = "agent.result.manual"
	AgentResultNotApplied Key = "agent.result.not_applied"
	AgentResultSync       Key = "agent.result.sync"
	AgentResultReleased   Key = "agent.result.released"
	AgentsDetailSync      Key = "agents.detail.sync"
	AgentsSyncInstalls    Key = "agents.sync.installs"
	AgentsSyncMissing     Key = "agents.sync.missing"
	AgentsSyncSkips       Key = "agents.sync.skips"
	AgentsSyncInactive    Key = "agents.sync.inactive"
	AgentsSyncUndeclared  Key = "agents.sync.undeclared"
	AgentsSyncCannot      Key = "agents.sync.cannot"
	AgentsSectionFound    Key = "agents.section.found"
	AgentsListHint        Key = "agents.list_hint"
	WizardWindowTitle     Key = "wizard.window_title"
	WizardCurrentDir      Key = "wizard.current_dir"
	WizardHintsHeading    Key = "wizard.hints_heading"
	WizardDiffHeading     Key = "wizard.diff_heading"
	WizardNextCommands    Key = "wizard.next_commands"
	WizardRefreshed       Key = "wizard.refreshed"
	WizardDiscardTitle    Key = "wizard.discard_title"
	WizardDiscardConfirm  Key = "wizard.discard_confirm"
)

// English returns the default catalog, including plain-language setting descriptions.
func English() Catalog {
	catalog := make(Catalog)
	for _, section := range [][]entry{statusText(), outcomeText(), driftText(), formText(), terminalText(), homeText(), agentsText(), wizardText(), settingDescriptions()} {
		for _, e := range section {
			catalog[e.key] = e.template
		}
	}
	return catalog
}

func statusText() []entry {
	return []entry{
		{ActionServiceRefresh, "Repair service"},
		{StatusVersions, "daemon %s, binary %s"},
		{StatusVersionMismatch, "Daemon/binary version mismatch; run kitbag service refresh."},
		{StatusServiceStale, "Service definition is stale; run kitbag service refresh."},
		{ServiceRefreshConfirm, "Refresh service and restart if needed? y confirm · n/Esc cancel"},
		{ServiceRefreshing, "Refreshing service…"},
		{ServiceRefreshed, "Service refreshed."},
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
		{StatusLastCheck, "Last check"},
		{StatusNextCheck, "Next check"},
		{StatusCheckUnknown, "No check recorded"},
		{StatusCheckOK, "ok"},
		{StatusCheckFailed, "failed: %s"},
		{StatusCheckValue, "%s, %s"},
		{StatusCheckStopped, "%s (stale; daemon not running)"},
		{StatusCheckStuck, "daemon is not checking"},
		{HelpCheckSync, "check = looked at the remote; sync = applied changes"},
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
		{SettingsAdvancedClosed, "Advanced [+]"},
		{SettingsAdvancedOpen, "Advanced [-]"},
		{SettingOn, "On"},
		{SettingOff, "Off"},
		{SettingCustom, "Custom…"},
		{SettingOther, "Other…"},
		{SettingGitValue, "From Git config: %s"},
		{SettingGitUnavailable, "Git config identity unavailable or not set"},
		{SettingLegacyCLI, "Edit old paths with kitbag config set migration.legacy_roots '<TOML array>'"},
		{SettingPickerHint, "←/→/Space or click: choose · Enter saves · Esc cancels"},
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
		{DialogEscape, "esc"},
		{DialogScroll, "scroll ↑/↓"},
		{DialogOK, "OK"},
		{DialogYes, "Yes"},
		{DialogNo, "No"},
		{DialogBack, "Back"},
		{DialogNext, "Next"},
		{DialogFinish, "Finish"},
		{DialogStep, "Step %s/%s"},
		{DialogHint, "Tab/↑/↓: focus · Enter: confirm · Esc/outside click: cancel"},
		{HelpDialogs, "Dialogs: Tab/arrows focus; Enter confirms; Esc/outside click cancels"},
		{SettingHelp, "Help"},
		{SettingCompactHint, "Enter saves · Esc cancels"},
		{SettingUnsetTitle, "Remove stored setting?"},
		{SettingUnsetConfirm, "Remove %s from config.toml? The effective value will be resolved again using the normal precedence."},
		{TerminalTooSmall, "Enlarge the terminal to at least %s×%s (now %s×%s)"},
		{TerminalFooter, "? help · a advanced · q quit · mouse: click, wheel"},
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
		{HelpTabs, "Home: s/a/d/c/l; 1–5: Status/Drift/Settings/Activity/Agents; i: Status"},
		{HelpRefresh, "r: reload status, drift, settings and agents"},
		{HelpSync, "s: sync now"},
		{HelpMove, "↑/↓ or k/j: move the selection"},
		{HelpDetail, "Enter: open drift details; Esc: close them"},
		{HelpEdit, "Enter: edit a setting, Enter again saves; Esc cancels"},
		{HelpSave, "w: save a kept draft; u: confirm removal from config.toml"},
		{HelpChoices, "←/→/Space or click while editing: choose; Custom/Other: type"},
		{HelpAdvanced, "a on Settings: expand or collapse Advanced"},
		{HelpFollow, "f: follow new activity; ↑/↓, PgUp/PgDn: scroll"},
		{HelpMouse, "Mouse: click home rows, breadcrumb or buttons; wheel: scroll"},
		{HelpToggle, "?: show or hide this help"},
		{HelpQuit, "q: home (quit on home); Ctrl-C: quit"},
		{HelpAgents, "Agents: a add, e or Enter edit, d remove, p look for more directories"},
		{FooterAgents, "? help · a add · e edit · d remove · p probe · q quit · mouse: click, wheel"},
		{DialogChoiceHint, "Space/Enter/click: choose · ←/→: previous/next choice"},
		{DialogScrollHint, "PgUp/PgDn or wheel: scroll"},
	}
}

func homeText() []entry {
	return []entry{
		{FooterSave, "Enter saves"},
		{FooterCancel, "Esc cancels"},
		{FooterHome, "q home"},
		{FooterQuit, "q quit"},
		{FooterHelp, "? help"},
		{FooterBack, "esc back"},
		{ActionDetails, "Details"},
		{ActionFollow, "Follow / pause"},
		{HomeBrand, "kitbag"},
		{HomeCount, "%s"},
		{HomeAgentCount, "%s agents"},
		{HomeVersion, "kitbag %s"},
		{HomeChecked, "%s · checked %s"},
		{HomeNextCheck, "Next check %s"},
		{HomeRepository, "%s · %s"},
		{HomeFooter, "%s · %s"},
		{HomeUnavailable, "unavailable"},
		{HomeCreatedBy, "Created by Tomasz Janusz"},
		{HomeMadeWith, "Made with ❤ in 🇵🇱"},
		{HomeMadeWithPlain, "Made with love in Poland"},
		{HomeSyncKey, "s"},
		{HomeAgentsKey, "a"},
		{HomeDriftKey, "d"},
		{HomeSettingsKey, "c"},
		{HomeActivityKey, "l"},
		{ScreenBreadcrumb, "kitbag › %s"},
		{ScreenFooter, "%s · %s · %s"},
		{ActionRemoveShort, "Remove"},
	}
}
