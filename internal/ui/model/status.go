package model

import (
	"strconv"

	"github.com/TomaszJanusz/kitbag/internal/timefmt"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Status tracks the status snapshot, this renderer's sync and live hints.
// Results carry sequence numbers: an older load never overwrites a newer one.
type Status struct {
	clock       timefmt.Clock
	problem     *Error
	syncProblem *Error
	outcome     *SyncOutcome
	syncPhase   string
	snapshot    StatusSnapshot
	issued      uint64
	applied     uint64
	// otherSince is the last load issued before a foreign sync started; a
	// later successful load proves the state lock was free again.
	otherSince uint64
	phase      Phase
	syncing    bool
	otherSync  bool
	update     bool
	live       bool
	known      bool
}

// NewStatus returns a loading model; run Init's commands to load it.
func NewStatus() *Status { return &Status{} }

// Init requests the first snapshot.
func (m *Status) Init() []Cmd { return m.load() }

func (m *Status) load() []Cmd {
	m.issued++
	return []Cmd{LoadStatus{Seq: m.issued}}
}

// Update applies msg and returns follow-up commands.
func (m *Status) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case Refresh:
		return m.load()
	case StatusLoaded:
		m.loaded(msg)
	case SyncNow:
		return m.startSync()
	case SyncProgressed:
		if m.syncing {
			m.syncPhase = msg.Phase
		}
	case SyncFinished:
		return m.finishSync(msg)
	case EventReceived:
		return m.event(msg.Event)
	case SubscriptionEnded:
		m.live = false
	}
	return nil
}

func (m *Status) loaded(msg StatusLoaded) {
	if msg.Seq <= m.applied || msg.Seq > m.issued {
		return
	}
	m.applied = msg.Seq
	if msg.Err != nil {
		m.problem = Classify(msg.Err)
		if msg.Snapshot.ToolTooOld != nil {
			m.snapshot, m.known = msg.Snapshot, true
		}
		if !m.known {
			m.phase = Failed
		}
		return
	}
	m.snapshot, m.known, m.problem, m.phase = msg.Snapshot, true, nil, Ready
	if m.otherSync && msg.Seq > m.otherSince {
		m.otherSync = false
	}
}

// canSync is false while any sync runs or the repository needs a newer tool.
func (m *Status) canSync() bool {
	return !m.syncing && !m.otherSync && m.snapshot.ToolTooOld == nil
}

func (m *Status) startSync() []Cmd {
	if !m.canSync() {
		return nil
	}
	m.syncing, m.syncPhase, m.syncProblem = true, "", nil
	return []Cmd{RunSync{}}
}

func (m *Status) finishSync(msg SyncFinished) []Cmd {
	if !m.syncing {
		return nil
	}
	outcome := msg.Outcome
	m.syncing, m.syncPhase, m.outcome, m.syncProblem = false, "", &outcome, Classify(msg.Err)
	return m.load()
}

func (m *Status) event(e Event) []Cmd {
	switch e.Kind {
	case EventConnected:
		// Hints may have been lost while disconnected; snapshots are authoritative.
		m.live = true
		return m.load()
	case EventSyncStarted:
		if !m.syncing {
			m.otherSync, m.otherSince = true, m.issued
		}
	case EventSyncFinished:
		m.otherSync = false
		return m.load()
	case EventPollFinished, EventDriftDetected:
		return m.load()
	case EventUpdateAvailable:
		m.update = true
	}
	return nil
}

// StatusView is everything a renderer shows for status.
type StatusView struct {
	Title    text.Message
	Headline text.Message
	Problem  *text.Message
	Notices  []text.Message
	Warnings []text.Message
	Rows     []Row
	Actions  []Control
	Phase    Phase
	Syncing  bool
}

// View projects the current state.
func (m *Status) View() StatusView {
	view := StatusView{Title: text.New(text.StatusTitle), Phase: m.phase, Syncing: m.syncing, Headline: m.headline(), Warnings: plain(m.snapshot.Warnings)}
	if m.problem != nil {
		problem := ProblemText(m.problem)
		if m.known {
			problem = text.New(text.StatusStale, problem)
		}
		view.Problem = &problem
	}
	view.Warnings = append(view.Warnings, m.checkWarnings()...)
	view.Notices = m.notices()
	if m.known {
		view.Rows = m.rows()
	}
	view.Actions = []Control{
		{Intent: SyncNow{}, Label: text.New(text.ActionSyncNow), Enabled: m.canSync()},
		{Intent: Refresh{}, Label: text.New(text.ActionRefresh), Enabled: true},
	}
	return view
}

func (m *Status) headline() text.Message {
	switch {
	case m.syncing:
		return text.New(text.StatusSyncing, PhaseText(m.syncPhase))
	case m.snapshot.ToolTooOld != nil && m.snapshot.ToolTooOld.Schema != 0:
		tooOld := m.snapshot.ToolTooOld
		return text.New(text.StatusSchemaTooNew, text.Plain(strconv.FormatInt(tooOld.Schema, 10)), text.Plain(tooOld.Current), text.Plain(strconv.FormatInt(tooOld.SupportedSchema, 10)))
	case m.snapshot.ToolTooOld != nil:
		return text.New(text.StatusToolTooOld, text.Plain(m.snapshot.ToolTooOld.Required), text.Plain(m.snapshot.ToolTooOld.Current))
	case !m.known && m.phase == Failed:
		return ProblemText(m.problem)
	case !m.known:
		return text.New(text.StatusLoading)
	case m.otherSync:
		return text.New(text.StatusOtherSync)
	default:
		return driftHeadline(m.snapshot.Drift)
	}
}

func driftHeadline(drift DriftCounts) text.Message {
	attention := drift.Conflict + drift.UnmanagedObstacle
	switch {
	case attention > 0:
		return text.New(text.StatusConflicts, number(attention))
	case drift.LocalDrift > 0:
		return text.New(text.StatusLocalDrift, number(drift.LocalDrift))
	case drift.UpdateNeeded > 0:
		return text.New(text.StatusUpdates, number(drift.UpdateNeeded))
	default:
		return text.New(text.StatusInSync)
	}
}

func (m *Status) notices() []text.Message {
	var notices []text.Message
	if m.update {
		notices = append(notices, text.New(text.StatusUpdate))
	}
	if m.outcome != nil {
		notices = append(notices, text.New(text.StatusSyncResult, ResultText(m.outcome.ExitCode), number(m.outcome.Changed)))
	}
	if m.syncProblem != nil {
		notices = append(notices, ProblemText(m.syncProblem))
	}
	return notices
}

func (m *Status) rows() []Row {
	s := m.snapshot
	branch := text.New(text.StatusNoUpstream, text.Plain(s.Git.Branch))
	if s.Git.Upstream != "" && !s.Git.UpstreamGone {
		branch = text.New(text.StatusBranchValue, text.Plain(s.Git.Branch), text.Plain(s.Git.Upstream), number(s.Git.Ahead), number(s.Git.Behind))
	}
	copyState := text.New(text.StatusClean)
	if s.Git.Dirty {
		copyState = text.New(text.StatusDirty)
	}
	last := text.New(text.StatusNeverSynced)
	if s.LastSync != nil {
		last = text.New(text.StatusLastSyncValue, WhenText(m.clock, s.LastSync.At), ResultText(s.LastSync.ExitCode), number(s.LastSync.Changed))
	}
	d := s.Drift
	rows := []Row{
		{Label: text.New(text.StatusRepository), Value: text.Plain(s.Repository)},
		{Label: text.New(text.StatusBranch), Value: branch},
		{Label: text.New(text.StatusWorkingCopy), Value: copyState},
		{Label: text.New(text.StatusLastSync), Value: last},
		{Label: text.New(text.StatusLastCheck), Value: CheckValue(m.clock, s.LastCheck, s.Daemon)},
		{Label: text.New(text.StatusDrift), Value: text.New(text.StatusDriftValue, number(d.InSync), number(d.UpdateNeeded), number(d.LocalDrift), number(d.Conflict), number(d.UnmanagedObstacle))},
		{Label: text.New(text.StatusConnection), Value: m.connection()},
	}
	if s.Daemon && s.LastCheck != nil {
		rows = append(rows, Row{Label: text.New(text.StatusNextCheck), Value: text.Plain(m.clock.Stamp(s.LastCheck.NextAt))})
	}
	return rows
}

func (m *Status) connection() text.Message {
	switch {
	case !m.snapshot.Daemon:
		return text.New(text.StatusLocal)
	case m.live:
		return text.New(text.StatusDaemonLive)
	default:
		return text.New(text.StatusDaemonQuiet)
	}
}
