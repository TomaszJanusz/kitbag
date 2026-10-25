package model

import "github.com/TomaszJanusz/kitbag/internal/ui/model/text"

// Drift lists targets other than in sync and details the selected one. The
// selection follows its path across reloads.
type Drift struct {
	problem  *Error
	selected string
	snapshot DriftSnapshot
	issued   uint64
	applied  uint64
	phase    Phase
	known    bool
}

// NewDrift returns a loading model; run Init's commands to load it.
func NewDrift() *Drift { return &Drift{} }

// Init requests the first plan projection.
func (m *Drift) Init() []Cmd { return m.load() }

func (m *Drift) load() []Cmd {
	m.issued++
	return []Cmd{LoadDrift{Seq: m.issued}}
}

// Update applies msg and returns follow-up commands.
func (m *Drift) Update(msg Msg) []Cmd {
	switch msg := msg.(type) {
	case Refresh, SyncFinished:
		return m.load()
	case DriftLoaded:
		m.loaded(msg)
	case SelectDrift:
		m.selectIndex(msg.Index)
	case MoveDrift:
		m.selectIndex(m.index() + msg.Delta)
	case EventReceived:
		switch msg.Event.Kind {
		case EventConnected, EventSyncFinished, EventDriftDetected:
			return m.load()
		case EventSyncStarted, EventUpdateAvailable:
		}
	}
	return nil
}

func (m *Drift) loaded(msg DriftLoaded) {
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

// index is the selected item's position, or -1.
func (m *Drift) index() int {
	for i, item := range m.snapshot.Items {
		if item.Path == m.selected {
			return i
		}
	}
	return -1
}

func (m *Drift) selectIndex(i int) {
	items := m.snapshot.Items
	if len(items) == 0 {
		m.selected = ""
		return
	}
	m.selected = items[min(max(i, 0), len(items)-1)].Path
}

// DriftRow is one list entry.
type DriftRow struct {
	Path           text.Message
	Classification text.Message
	Selected       bool
}

// DriftDetail describes the selected target's planned actions. Diff explains
// that content diffs are not part of the API.
type DriftDetail struct {
	Path           text.Message
	Classification text.Message
	Diff           text.Message
	Actions        []text.Message
}

// DriftView is everything a renderer shows for drift.
type DriftView struct {
	Detail   *DriftDetail
	Problem  *text.Message
	Title    text.Message
	Headline text.Message
	Rows     []DriftRow
	Warnings []text.Message
	Phase    Phase
}

// View projects the current state.
func (m *Drift) View() DriftView {
	view := DriftView{Title: text.New(text.DriftTitle), Phase: m.phase, Headline: m.headline(), Warnings: plain(m.snapshot.Warnings)}
	if m.problem != nil {
		problem := ProblemText(m.problem)
		view.Problem = &problem
	}
	for _, item := range m.snapshot.Items {
		selected := item.Path == m.selected
		view.Rows = append(view.Rows, DriftRow{Path: text.Plain(item.Path), Classification: ClassificationText(item.Classification), Selected: selected})
		if selected {
			view.Detail = detail(item)
		}
	}
	return view
}

func (m *Drift) headline() text.Message {
	switch {
	case !m.known && m.phase == Failed:
		return ProblemText(m.problem)
	case !m.known:
		return text.New(text.DriftLoading)
	case len(m.snapshot.Items) == 0:
		return text.New(text.DriftEmpty)
	default:
		return text.New(text.DriftCount, number(len(m.snapshot.Items)))
	}
}

func detail(item DriftItem) *DriftDetail {
	result := &DriftDetail{Path: text.Plain(item.Path), Classification: ClassificationText(item.Classification), Diff: text.New(text.DriftNoDiff)}
	for _, action := range item.Actions {
		result.Actions = append(result.Actions, actionText(action.Kind))
		if action.SourcePath != "" {
			result.Actions = append(result.Actions, text.New(text.OpSource, text.Plain(action.SourcePath)))
		}
		if action.BackupPath != "" {
			result.Actions = append(result.Actions, text.New(text.OpBackupPath, text.Plain(action.BackupPath)))
		}
		if action.Warning != "" {
			result.Actions = append(result.Actions, text.New(text.OpWarning, text.Plain(action.Warning)))
		}
	}
	if len(item.Actions) == 0 {
		result.Actions = []text.Message{text.New(text.DriftNoOperation)}
	}
	return result
}

func actionText(kind ActionKind) text.Message {
	keys := map[ActionKind]text.Key{
		ActionRemove: text.OpRemove, ActionDiscard: text.OpDiscard, ActionBackup: text.OpBackup,
		ActionMaterialize: text.OpMaterialize, ActionWrite: text.OpWrite, ActionMerge: text.OpMerge,
		ActionUnmerge: text.OpUnmerge, ActionRestore: text.OpRestore, ActionRecord: text.OpRecord,
		ActionRelease: text.OpRelease, ActionSkip: text.OpSkip,
	}
	if key, ok := keys[kind]; ok {
		return text.New(key)
	}
	return text.New(text.OpOther, text.Plain(string(kind)))
}
