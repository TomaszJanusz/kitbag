package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func (m *Model) statusView() []string {
	view := m.app.Status.View()
	headline := m.styles.heading
	if view.Syncing {
		headline = m.styles.success
	}
	lines := m.wrap(headline, m.say(view.Headline))
	lines = append(lines, m.problem(view.Problem, view.Headline)...)
	for _, notice := range view.Notices {
		lines = append(lines, m.wrap(m.styles.warning, m.say(notice))...)
	}
	if len(view.Rows) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.rows(view.Rows)...)
	}
	lines = append(lines, m.warnings(view.Warnings)...)
	return lines
}

// rows aligns labels; values wrap below their label on narrow terminals.
func (m *Model) rows(rows []model.Row) []string {
	labels := make([]string, len(rows))
	widest := 0
	for i, row := range rows {
		labels[i] = m.say(row.Label)
		widest = max(widest, ansi.StringWidth(labels[i]))
	}
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		label := labels[i] + strings.Repeat(" ", widest-ansi.StringWidth(labels[i]))
		value := strings.Split(ansi.Wrap(m.say(row.Value), max(m.width-widest-len(indent), 1), ""), "\n")
		lines = append(lines, m.styles.selected.Render(label)+indent+value[0])
		for _, more := range value[1:] {
			lines = append(lines, strings.Repeat(" ", widest)+indent+more)
		}
	}
	return lines
}

// listLayout places a scrolling list: head lines, a blank separator, the
// visible rows starting at index first, then tail lines. Drawing and mouse
// hit-testing both use it.
type listLayout struct {
	head, tail  []string
	first, rows int
}

// top is the screen line of the first visible row (below the tab bar and its
// separator, the head lines and the blank line).
func (l listLayout) top() int { return chromeLines - 1 + len(l.head) + 1 }

func (m *Model) driftLayout(height int) listLayout {
	view := m.app.Drift.View()
	head := m.wrap(m.styles.heading, m.say(view.Headline))
	head = append(head, m.problem(view.Problem, view.Headline)...)
	tail := m.warnings(view.Warnings)
	if len(view.Rows) > 0 {
		tail = append(tail, "", m.styles.faint.Render(m.say(text.New(text.DriftListHint))))
	}
	space := height - len(head) - 1 - len(tail)
	selected := 0
	for i, row := range view.Rows {
		if row.Selected {
			selected = i
		}
	}
	first := window(selected, len(view.Rows), space)
	return listLayout{head: head, tail: tail, first: first, rows: min(max(space, 1), len(view.Rows)-first)}
}

func (m *Model) driftView(height int) []string {
	view := m.app.Drift.View()
	if m.detail && view.Detail != nil {
		return m.driftDetail(*view.Detail)
	}
	layout := m.driftLayout(height)
	lines := layout.head
	if len(view.Rows) > 0 {
		lines = append(lines, "")
	}
	for _, row := range view.Rows[layout.first : layout.first+layout.rows] {
		lines = append(lines, m.listRow(row.Selected, m.say(row.Path), m.say(row.Classification)))
	}
	return append(lines, layout.tail...)
}

func (m *Model) listRow(selected bool, name, detail string) string {
	prefix, style := indent, m.styles.tab
	if selected {
		prefix, style = marker, m.styles.selected
	}
	return style.Render(prefix+name) + indent + m.styles.faint.Render(detail)
}

func (m *Model) driftDetail(detail model.DriftDetail) []string {
	lines := m.wrap(m.styles.heading, m.say(detail.Path))
	lines = append(lines, m.wrap(m.styles.warning, m.say(detail.Classification))...)
	lines = append(lines, "", m.styles.selected.Render(m.say(text.New(text.DriftActions))))
	for _, action := range detail.Actions {
		lines = append(lines, m.wrap(m.styles.tab, bullet+m.say(action))...)
	}
	lines = append(lines, "")
	lines = append(lines, m.wrap(m.styles.faint, m.say(detail.Diff))...)
	return append(lines, "", m.styles.faint.Render(m.say(text.New(text.DriftDetailHint))))
}

// logRows is the number of entries the log tab shows at the current size.
func (m *Model) logRows() int { return max(m.bodyHeight()-1, 1) }

func (m *Model) logView(height int) []string {
	view := m.app.Log.View()
	state := text.New(text.LogFollowing)
	if !m.follow {
		state = text.New(text.LogPaused)
	}
	status := m.say(state)
	if view.Dropped > 0 {
		status += " · " + m.say(text.New(text.LogDropped, number(view.Dropped)))
	}
	lines := []string{m.styles.faint.Render(status)}
	if len(view.Entries) == 0 {
		return append(lines, m.say(text.New(text.LogEmpty)))
	}
	rows := height - 1
	first := m.logTop
	if m.follow {
		first = max(len(view.Entries)-rows, 0)
	}
	first = min(first, max(len(view.Entries)-rows, 0))
	for _, entry := range view.Entries[first:min(first+rows, len(view.Entries))] {
		lines = append(lines, m.logLine(entry))
	}
	return lines
}

func (m *Model) logLine(entry model.LogEntry) string {
	level, style := text.New(text.LogLevelInfo), m.styles.tab
	switch entry.Severity {
	case model.Warning:
		level, style = text.New(text.LogLevelWarning), m.styles.warning
	case model.Failure:
		level, style = text.New(text.LogLevelFailure), m.styles.failure
	case model.Info:
	}
	return m.styles.faint.Render(entry.Time) + " " + style.Render(m.say(level)+" "+m.say(entry.Text))
}

func (m *Model) editorHint(field model.FieldView) []string {
	key := text.SettingPickerHint
	if field.TextEntry {
		key = text.SettingEditing
	}
	return m.wrap(m.styles.faint, m.say(text.New(key)))
}
