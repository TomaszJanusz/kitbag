package model

import (
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Agents lists declared instances grouped by agent kind, then undeclared
// directories of that kind. The selection follows (kind, directory) across
// reloads.
type Agents struct {
	problem  *Error
	selected agentKey
	snapshot AgentsSnapshot
	issued   uint64
	applied  uint64
	phase    Phase
	known    bool
}

type agentKey struct{ kind, dir string }

// agentEntry is one listed directory; instance is nil for a candidate.
type agentEntry struct {
	instance *AgentInstance
	AgentCandidate
}

// NewAgents returns a loading model; run Init's commands to load it.
func NewAgents() *Agents { return &Agents{} }

// Init requests the first listing.
func (m *Agents) Init() []Cmd { return m.load(false) }

func (m *Agents) load(probe bool) []Cmd {
	m.issued++
	return []Cmd{LoadAgents{Seq: m.issued, Probe: probe}}
}

// Update applies msg and returns follow-up commands.
func (m *Agents) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case Refresh, SyncFinished:
		return m.load(false)
	case ProbeAgents:
		return m.load(true)
	case AgentsLoaded:
		m.loaded(msg)
	case SelectAgent:
		m.selectIndex(msg.Index)
	case MoveAgent:
		m.selectIndex(m.index() + msg.Delta)
	case AgentChangeApplied:
		if msg.Err == nil {
			return m.load(false)
		}
	case EventReceived:
		return m.event(msg.Event)
	}
	return nil
}

// event reloads after a connection or a sync, which may change instances.
func (m *Agents) event(event Event) []Cmd {
	switch event.Kind {
	case EventConnected, EventSyncFinished:
		return m.load(false)
	case EventPollFinished, EventSyncStarted, EventDriftDetected, EventUpdateAvailable:
	}
	return nil
}

func (m *Agents) loaded(msg AgentsLoaded) {
	if msg.Seq <= m.applied || msg.Seq > m.issued {
		return
	}
	m.applied = msg.Seq
	if msg.Err != nil {
		m.problem = Classify(msg.Err)
		if !m.known {
			m.phase = Failed
		}
		return
	}
	m.snapshot, m.known, m.problem, m.phase = msg.Snapshot, true, nil, Ready
	if m.index() < 0 {
		m.selectIndex(0)
	}
}

// Snapshot returns the last successful listing.
func (m *Agents) Snapshot() AgentsSnapshot { return m.snapshot }

// entries orders instances then candidates per kind, in the order of Kinds;
// kinds missing from Kinds follow.
func (m *Agents) entries() []agentEntry {
	kinds := append([]string{}, m.snapshot.Kinds...)
	add := func(kind string) {
		for _, known := range kinds {
			if known == kind {
				return
			}
		}
		kinds = append(kinds, kind)
	}
	for _, i := range m.snapshot.Instances {
		add(i.Kind)
	}
	for _, c := range m.snapshot.Candidates {
		add(c.Kind)
	}
	var result []agentEntry
	for _, kind := range kinds {
		for i := range m.snapshot.Instances {
			if instance := &m.snapshot.Instances[i]; instance.Kind == kind {
				result = append(result, agentEntry{instance: instance, AgentCandidate: instance.AgentCandidate})
			}
		}
		for _, c := range m.snapshot.Candidates {
			if c.Kind == kind {
				result = append(result, agentEntry{AgentCandidate: c})
			}
		}
	}
	return result
}

// index is the selected entry's position, or -1.
func (m *Agents) index() int {
	for i, e := range m.entries() {
		if (agentKey{e.Kind, e.Dir}) == m.selected {
			return i
		}
	}
	return -1
}

func (m *Agents) selectIndex(i int) {
	entries := m.entries()
	if len(entries) == 0 {
		m.selected = agentKey{}
		return
	}
	e := entries[min(max(i, 0), len(entries)-1)]
	m.selected = agentKey{e.Kind, e.Dir}
}

// AgentRow is one listed directory. Label is target/name, the implicit
// default of a target, or "not declared"; Scope is empty for candidates.
type AgentRow struct {
	Kind, Target, Name                  string
	Label, Dir, Source, Presence, Scope text.Message
	Declared, Implicit, ActiveHere      bool
	Selected                            bool
}

// AgentDetail describes the selected row.
type AgentDetail struct {
	Title text.Message
	Rows  []Row
}

// AgentsView is everything a renderer shows for agents. Rows are grouped by
// Kind in display order; a renderer starts a group where Kind changes.
type AgentsView struct {
	Detail   *AgentDetail
	Problem  *text.Message
	Title    text.Message
	Headline text.Message
	Rows     []AgentRow
	Warnings []text.Message
	Actions  []Control
	Phase    Phase
}

// View projects the current state.
func (m *Agents) View() AgentsView {
	view := AgentsView{Title: text.New(text.AgentsTitle), Phase: m.phase, Headline: m.headline(), Warnings: plain(m.snapshot.Warnings)}
	if m.problem != nil {
		problem := ProblemText(m.problem)
		view.Problem = &problem
	}
	var selected *agentEntry
	for _, e := range m.entries() {
		row := agentRow(e)
		row.Selected = (agentKey{e.Kind, e.Dir}) == m.selected
		if row.Selected {
			selected = &e
			view.Detail = &AgentDetail{Title: row.Label, Rows: agentDetailRows(e, row)}
		}
		view.Rows = append(view.Rows, row)
	}
	view.Actions = agentActions(selected)
	return view
}

func (m *Agents) headline() text.Message {
	declared := 0
	for _, i := range m.snapshot.Instances {
		if !i.Implicit {
			declared++
		}
	}
	switch {
	case !m.known && m.phase == Failed:
		return ProblemText(m.problem)
	case !m.known:
		return text.New(text.AgentsLoading)
	case len(m.snapshot.Instances) == 0 && len(m.snapshot.Candidates) == 0:
		return text.New(text.AgentsEmpty)
	default:
		return text.New(text.AgentsCount, number(declared), number(len(m.snapshot.Candidates)))
	}
}

func agentRow(e agentEntry) AgentRow {
	row := AgentRow{Kind: e.Kind, Dir: text.Plain(e.Dir), Source: AgentSourceText(e.Source, e.Variable), Presence: AgentPresenceText(e.Presence), Label: text.New(text.AgentsUndeclared)}
	if i := e.instance; i != nil {
		row.Target, row.Name, row.Implicit, row.ActiveHere = i.Target, i.Name, i.Implicit, i.ActiveHere
		row.Declared, row.Label, row.Scope = !i.Implicit, text.Plain(i.Target+"/"+i.Name), agentScope(*i)
		if i.Implicit {
			row.Label = text.New(text.AgentsImplicit, text.Plain(i.Target))
		}
	}
	return row
}

func agentScope(i AgentInstance) text.Message {
	switch {
	case len(i.Hosts) > 0:
		return text.New(text.AgentsScopeHosts, text.Plain(strings.Join(i.Hosts, ", ")))
	case len(i.ExcludedHosts) > 0:
		return text.New(text.AgentsScopeExcept, text.Plain(strings.Join(i.ExcludedHosts, ", ")))
	default:
		return text.New(text.AgentsScopeAll)
	}
}

func agentDetailRows(e agentEntry, row AgentRow) []Row {
	rows := []Row{
		{Label: text.New(text.AgentsDetailDir), Value: row.Dir},
		{Label: text.New(text.AgentsDetailSource), Value: row.Source},
		{Label: text.New(text.AgentsDetailPresence), Value: row.Presence},
	}
	if i := e.instance; i != nil {
		state, settings := text.New(text.AgentsInactiveHere), text.New(text.AgentsSettingsOff)
		if i.ActiveHere {
			state = text.New(text.AgentsActiveHere)
		}
		if i.Settings {
			settings = text.New(text.AgentsSettingsOn)
		}
		rows = append(rows, Row{Label: text.New(text.AgentsDetailScope), Value: row.Scope},
			Row{Label: text.New(text.AgentsDetailState), Value: state},
			Row{Label: text.New(text.AgentsDetailSettings), Value: settings})
	}
	return append(rows, Row{Label: text.New(text.AgentsDetailSync), Value: agentSyncText(e)})
}

// agentSyncText says what the next sync does with the directory.
func agentSyncText(e agentEntry) text.Message {
	i := e.instance
	switch {
	case i == nil && e.Presence == PresenceOutsideHome:
		return text.New(text.AgentsSyncCannot)
	case i == nil:
		return text.New(text.AgentsSyncUndeclared)
	case !i.ActiveHere:
		return text.New(text.AgentsSyncInactive)
	case e.Presence == PresencePresent:
		return text.New(text.AgentsSyncInstalls)
	case e.Presence == PresenceMissing:
		return text.New(text.AgentsSyncMissing)
	default:
		return text.New(text.AgentsSyncSkips)
	}
}

// agentActions offers add (prefilled from a selected candidate), edit and
// remove of a selected declared instance, the name probe and refresh.
func agentActions(e *agentEntry) []Control {
	add := OpenAgentForm{Op: AgentAdd}
	edit, remove := OpenAgentForm{Op: AgentEdit}, OpenAgentForm{Op: AgentRemove}
	declared := false
	if e != nil {
		add.Kind = e.Kind
		if e.instance == nil {
			add.Dir = e.Dir
		}
		if i := e.instance; i != nil && !i.Implicit {
			declared = true
			edit = OpenAgentForm{Op: AgentEdit, Kind: i.Kind, Target: i.Target, Name: i.Name, Dir: i.Dir}
			remove = OpenAgentForm{Op: AgentRemove, Kind: i.Kind, Target: i.Target, Name: i.Name, Dir: i.Dir}
		}
	}
	actions := []Control{{Intent: add, Label: text.New(text.ActionAddAgent), Enabled: true}}
	if declared {
		actions = append(actions,
			Control{Intent: edit, Label: text.New(text.ActionEditAgent), Enabled: true},
			Control{Intent: remove, Label: text.New(text.ActionRemoveAgent), Enabled: true})
	}
	return append(actions,
		Control{Intent: ProbeAgents{}, Label: text.New(text.ActionProbeAgents), Enabled: true},
		Control{Intent: Refresh{}, Label: text.New(text.ActionRefresh), Enabled: true})
}

// AgentSourceText describes where a directory comes from.
func AgentSourceText(source AgentSource, variable string) text.Message {
	keys := map[AgentSource]text.Key{
		AgentSourceDefault: text.AgentsSourceDefault, AgentSourceEnvironment: text.AgentsSourceEnv,
		AgentSourceDeclared: text.AgentsSourceDeclared, AgentSourceProbe: text.AgentsSourceProbe,
	}
	key, ok := keys[source]
	switch {
	case !ok:
		return text.Plain(string(source))
	case source == AgentSourceEnvironment:
		return text.New(key, text.Plain(variable))
	default:
		return text.New(key)
	}
}

// AgentPresenceText describes what was found at a directory.
func AgentPresenceText(presence AgentPresence) text.Message {
	keys := map[AgentPresence]text.Key{
		PresencePresent: text.AgentsPresent, PresenceMissing: text.AgentsMissing, PresenceNotDirectory: text.AgentsNotDirectory,
		PresenceSymlink: text.AgentsSymlink, PresenceOutsideHome: text.AgentsOutsideHome, PresenceUnreadable: text.AgentsUnreadable,
	}
	if key, ok := keys[presence]; ok {
		return text.New(key)
	}
	return text.Plain(string(presence))
}
