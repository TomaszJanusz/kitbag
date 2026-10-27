package model

import "time"

// AgentPresence is what a single Lstat found at an agent directory.
type AgentPresence string

// Directory presence values; only PresencePresent is a usable directory.
const (
	PresencePresent      AgentPresence = "present"
	PresenceMissing      AgentPresence = "missing"
	PresenceNotDirectory AgentPresence = "not_directory"
	PresenceSymlink      AgentPresence = "symlink"
	PresenceOutsideHome  AgentPresence = "outside_home"
	PresenceUnreadable   AgentPresence = "unreadable"
)

// AgentSource says where a directory comes from.
type AgentSource string

// Directory sources.
const (
	AgentSourceDefault     AgentSource = "default"
	AgentSourceEnvironment AgentSource = "environment"
	AgentSourceDeclared    AgentSource = "declared"
	AgentSourceProbe       AgentSource = "probe"
)

// AgentCandidate is a directory of one agent kind. Dir is absolute; Variable
// names the relocation variable when Source is AgentSourceEnvironment.
type AgentCandidate struct {
	Kind, Dir, Variable string
	Source              AgentSource
	Presence            AgentPresence
}

// AgentInstance is a declared or implicit instance of a target. Empty Hosts
// means every host except ExcludedHosts (schema-1 replacement sets).
type AgentInstance struct {
	Target, Name string
	AgentCandidate
	Hosts, ExcludedHosts []string
	Implicit             bool
	Enabled              bool
	Settings             bool
	ActiveHere           bool
}

// AgentsSnapshot is one agents.list result.
type AgentsSnapshot struct {
	Host       string
	KnownHosts []string
	// Kinds are the native kinds plus kinds only declared (such as generic).
	Kinds      []string
	Instances  []AgentInstance
	Candidates []AgentCandidate
	Warnings   []string
	// Schema is the manifest schema, 0 without a manifest.
	Schema int
	Daemon bool
}

// AgentOp is the kind of instance change.
type AgentOp string

// Instance changes.
const (
	AgentAdd    AgentOp = "add"
	AgentEdit   AgentOp = "edit"
	AgentRemove AgentOp = "remove"
)

// AgentChange is a requested instance change. Add needs Kind, Name and Dir
// (~/..., or an absolute path below HOME); edit and remove need Target (a
// target or a kind with one target) and Name. Settings nil keeps the default
// (add) or the current value (edit).
type AgentChange struct {
	Settings *bool
	Op       AgentOp
	Kind     string
	Target   string
	Name     string
	Dir      string
	// Hosts restricts the instance to these hosts, separated by commas (a
	// string keeps changes comparable); AllHosts (edit) removes the
	// restriction.
	Hosts    string
	AllHosts bool
}

// PublishState says what happened to a change in Git.
type PublishState string

// Publication states.
const (
	// PublishDryRun is a preview; nothing was written.
	PublishDryRun PublishState = "dry_run"
	// PublishPublished means the commit is on the remote branch.
	PublishPublished PublishState = "published"
	// PublishNotPublished means the commit exists only in the working copy.
	PublishNotPublished PublishState = "not_published"
	// PublishManual means the host's mode is pr: nothing was written and
	// Next lists how to propose the change.
	PublishManual PublishState = "manual"
	// PublishConflict means the change no longer applies upstream.
	PublishConflict PublishState = "conflict"
)

// AgentPublication is the commit and its route: Mode is direct or pr.
type AgentPublication struct {
	State                        PublishState
	Mode, Branch, Commit, Reason string
	Next                         []string
	Attempts                     int
}

// AgentEffect is one planned filesystem consequence, without content.
type AgentEffect struct {
	Path, Kind, Backup, Warning string
	Existing                    bool
}

// AgentPreview is a planned change with the authoritative manifest path and
// filesystem effects. Pass its Revision to ApplyAgentChange.
type AgentPreview struct {
	Manifest, Directory string
	Effects             []AgentEffect
	Change              AgentChange
	Revision            string
	Title               string
	Message             string
	// Dropped is the ~/ directory the change stops declaring, if any.
	Dropped     string
	Diff        []string
	Warnings    []string
	Publication AgentPublication
}

// AgentRelease reports a dropped directory handed back by unmanage rules.
type AgentRelease struct {
	Dir               string
	Changed, Warnings []string
}

// AgentSyncReport is the sync run after publishing.
type AgentSyncReport struct {
	Warnings          []string
	ExitCode, Changed int
}

// AgentApplied is the outcome of ApplyAgentChange. ExitCode is the CLI's.
type AgentApplied struct {
	Released *AgentRelease
	Sync     *AgentSyncReport
	AgentPreview
	ExitCode int
}

// Agent intents.
type (
	// ProbeAgents reloads with the explicit HOME name probe.
	ProbeAgents struct{}
	// SelectAgent selects the agents row at Index.
	SelectAgent struct{ Index int }
	// MoveAgent moves the agents selection by Delta, clamped to the list.
	MoveAgent struct{ Delta int }
	// OpenAgentForm starts the form for Op, prefilled from the fields given
	// (for example a candidate's Kind and Dir, or an instance to edit).
	OpenAgentForm struct {
		Op                      AgentOp
		Kind, Target, Name, Dir string
		Adopt                   bool
	}
	// EditAgentField replaces the current step's text (name or directory).
	EditAgentField struct{ Value string }
	// ChooseAgentOption selects the option at Index on the current step
	// (agent kind, directory hint or settings choice).
	ChooseAgentOption struct{ Index int }
	// AgentFormNext advances when the current step is valid; on the confirm
	// step it applies.
	AgentFormNext struct{}
	// AgentFormBack returns to the previous step; on the first step it
	// cancels like CancelAgentForm.
	AgentFormBack struct{}
	// CancelAgentForm closes the form; with a draft it first asks, and a
	// second CancelAgentForm discards.
	CancelAgentForm struct{}
	// ResumeAgentForm answers the discard question with no: the draft stays.
	ResumeAgentForm struct{}
)

// Agent command results.
type (
	// AgentsLoaded carries an agents.list result.
	AgentsLoaded struct {
		Err      error
		Snapshot AgentsSnapshot
		Seq      uint64
	}
	// AgentPreviewed carries a dry-run result for the form's draft Seq.
	AgentPreviewed struct {
		Err     error
		Preview AgentPreview
		Seq     uint64
	}
	// AgentChangeApplied reports ApplyAgentChange.
	AgentChangeApplied struct {
		At      time.Time
		Err     error
		Applied AgentApplied
	}
)

// Agent commands.
type (
	// LoadAgents calls Client.Agents.
	LoadAgents struct {
		Seq   uint64
		Probe bool
	}
	// PreviewAgent calls Client.PreviewAgentChange.
	PreviewAgent struct {
		Change AgentChange
		Seq    uint64
	}
	// ApplyAgent calls Client.ApplyAgentChange.
	ApplyAgent struct {
		Revision string
		Change   AgentChange
	}
)

func (ProbeAgents) isMsg()        {}
func (SelectAgent) isMsg()        {}
func (MoveAgent) isMsg()          {}
func (OpenAgentForm) isMsg()      {}
func (EditAgentField) isMsg()     {}
func (ChooseAgentOption) isMsg()  {}
func (AgentFormNext) isMsg()      {}
func (AgentFormBack) isMsg()      {}
func (CancelAgentForm) isMsg()    {}
func (ResumeAgentForm) isMsg()    {}
func (AgentsLoaded) isMsg()       {}
func (AgentPreviewed) isMsg()     {}
func (AgentChangeApplied) isMsg() {}

func (LoadAgents) isCmd()   {}
func (PreviewAgent) isCmd() {}
func (ApplyAgent) isCmd()   {}
