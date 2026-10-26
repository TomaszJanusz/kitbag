package tui

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// agentsScreen is the drawn Agents screen with the cells clicks resolve to:
// rows maps a screen line to a list row.
type agentsScreen struct {
	rows  map[int]int
	lines []string
	panes PaneLayout
}

// agentLine is one list line; row is the AgentsView row it draws, or -1
// for a group heading.
type agentLine struct {
	text string
	row  int
}

// Stacked panes give the list at most half of the body and at least this
// many lines.
const (
	stackedListShare = 2
	stackedListMin   = 4
)

func (m *Model) agentsScreen(height int) agentsScreen {
	view := m.app.Agents.View()
	panes := MasterDetail(m.width, height, 0, max(height/stackedListShare, stackedListMin))
	local := *m
	local.width = panes.Master.Width
	head := local.agentHeading(view, height)
	list := m.agentLines(view.Rows, panes.Master.Width)
	detail := m.agentDetail(view, panes.Detail.Width)
	rows := max(panes.Master.Height-len(head)-1, 1)
	selected := 0
	for i, line := range list {
		if line.row >= 0 && view.Rows[line.row].Selected {
			selected = i
		}
	}
	first := window(selected, len(list), rows)
	left := append(append([]string{}, head...), "")
	hits := map[int]int{}
	top := chromeLines - 1 + panes.Master.Y + len(left)
	for i, line := range list[first:min(first+rows, len(list))] {
		left = append(left, line.text)
		if line.row >= 0 {
			hits[top+i] = line.row
		}
	}

	return agentsScreen{lines: composePanes(panes, left, detail), rows: hits, panes: panes}
}

// agentLines groups rows by kind under a heading and starts a "found, not
// declared" section before a kind's first undeclared directory. Each row
// takes two lines: the label and directory, then its states.
func (m *Model) agentLines(rows []model.AgentRow, width int) []agentLine {
	var lines []agentLine
	for i, row := range rows {
		newKind := i == 0 || rows[i-1].Kind != row.Kind
		if newKind {
			lines = append(lines, agentLine{text: m.styles.heading.Render(ansi.Truncate(row.Kind, width, ellipsis)), row: -1})
		}
		if candidate(row) && (newKind || !candidate(rows[i-1])) {
			lines = append(lines, agentLine{text: m.styles.faint.Render(ansi.Truncate(indent+"["+m.say(text.New(text.AgentsSectionFound))+"]", width, ellipsis)), row: -1})
		}
		first, states := m.agentRow(row)
		lines = append(lines,
			agentLine{text: ansi.Truncate(first, width, ellipsis), row: i},
			agentLine{text: m.styles.faint.Render(ansi.Truncate(indent+indent+states, width, ellipsis)), row: i})
	}
	return lines
}

// candidate reports a found directory that no instance declares.
func candidate(row model.AgentRow) bool { return !row.Declared && !row.Implicit }

// agentRow is a row's first line (marker, label and directory; a candidate
// shows only its directory) and its plain states line.
func (m *Model) agentRow(row model.AgentRow) (first, states string) {
	prefix, style := indent, m.styles.tab
	if row.Selected {
		prefix, style = marker, m.styles.selected
	}
	states = m.say(row.Presence) + " · " + m.say(row.Source)
	if candidate(row) {
		return style.Render(prefix + m.say(row.Dir)), states
	}
	states += " · " + m.say(row.Scope)
	if row.ActiveHere {
		states += " · " + m.say(text.New(text.AgentsActiveHere))
	}
	return style.Render(prefix+m.say(row.Label)) + indent + m.say(row.Dir), states
}

// agentHeading reserves list space on a short terminal once rows are loaded.
func (m *Model) agentHeading(view model.AgentsView, height int) []string {
	if height < compactHeight && len(view.Rows) > 0 && view.Problem == nil {
		return nil
	}
	head := m.wrap(m.styles.heading, m.say(view.Headline))
	return append(head, m.problem(view.Problem, view.Headline)...)
}

// agentDetail describes the row; actions use the shared fixed bottom row.
func (m *Model) agentDetail(view model.AgentsView, width int) []string {
	local := *m
	local.width = width
	var lines []string
	if detail := view.Detail; detail != nil {
		lines = append(lines, local.wrap(m.styles.heading, m.say(detail.Title))...)
		lines = append(lines, local.rows(detail.Rows)...)
	}
	lines = append(lines, local.wrap(m.styles.faint, m.say(text.New(text.AgentsListHint)))...)
	return append(lines, local.warnings(view.Warnings)...)
}

// clickAgents selects a clicked row; clicking the selected row again edits
// it, like Enter. The shared screen action row handles buttons.
func (m *Model) clickAgents(x, y int) {
	screen := m.agentsScreen(m.bodyHeight())
	view := m.app.Agents.View()

	if !screen.panes.Master.Contains(x, y-(chromeLines-1)) {
		return
	}
	row, ok := screen.rows[y]
	switch {
	case !ok:
	case view.Rows[row].Selected:
		m.agentsKey(keyEdit)
	default:
		m.dispatch(model.SelectAgent{Index: row})
	}
}
