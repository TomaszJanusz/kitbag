package tui

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// agentsScreen is the drawn Agents tab with the cells clicks resolve to:
// rows maps a screen line to a list row, actions are the action buttons.
type agentsScreen struct {
	lines   []string
	rows    map[int]int
	actions []dialogHit
	panes   PaneLayout
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
	head := local.wrap(m.styles.heading, m.say(view.Headline))
	head = append(head, local.problem(view.Problem, view.Headline)...)
	list := m.agentLines(view.Rows, panes.Master.Width)
	detail, actions := m.agentDetail(view, panes.Detail.Width, panes.Stacked)
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
	for i := range actions {
		actions[i].rect.X += panes.Detail.X
		actions[i].rect.Y += chromeLines - 1 + panes.Detail.Y
	}
	return agentsScreen{lines: composePanes(panes, left, detail), rows: hits, actions: actions, panes: panes}
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

// agentDetail describes the selected row, then the clickable actions, the
// key hint and the listing's warnings. Stacked below the list, where the
// pane is short, the actions come right after the title.
func (m *Model) agentDetail(view model.AgentsView, width int, stacked bool) ([]string, []dialogHit) {
	local := *m
	local.width = width
	var title, rows []string
	if detail := view.Detail; detail != nil {
		title = local.wrap(m.styles.heading, m.say(detail.Title))
		rows = local.rows(detail.Rows)
	}
	actions, hits := m.actionButtons(view.Actions, width)
	lines := append([]string{}, title...)
	if !stacked {
		lines = append(lines, rows...)
		if len(rows) > 0 {
			lines = append(lines, "")
		}
	}
	for i := range hits {
		hits[i].rect.Y += len(lines)
	}
	lines = append(lines, actions...)
	if stacked {
		lines = append(lines, rows...)
	}
	lines = append(lines, local.wrap(m.styles.faint, m.say(text.New(text.AgentsListHint)))...)
	return append(lines, local.warnings(view.Warnings)...), hits
}

// actionButtons lays out "[key] label" buttons, wrapping at width; hit
// indexes point into controls. Disabled actions say so in text.
func (m *Model) actionButtons(controls []model.Control, width int) ([]string, []dialogHit) {
	lines := []string{""}
	hits := make([]dialogHit, 0, len(controls))
	x := 0
	for i, c := range controls {
		label := m.say(text.New(text.ControlHint, text.Plain(intentName(c.Intent)), c.Label))
		style := m.styles.tab
		if !c.Enabled {
			label, style = m.say(text.New(text.ControlDisabled, text.Plain(label))), m.styles.faint
		}
		size := min(ansi.StringWidth(label), width)
		if x > 0 && x+len(indent)+size > width {
			lines = append(lines, "")
			x = 0
		}
		if x > 0 {
			lines[len(lines)-1] += indent
			x += len(indent)
		}
		hits = append(hits, dialogHit{rect: Rect{X: x, Y: len(lines) - 1, Width: size, Height: 1}, index: i})
		lines[len(lines)-1] += style.Render(ansi.Truncate(label, width, ellipsis))
		x += size
	}
	return lines, hits
}

// clickAgents selects a clicked row; clicking the selected row again edits
// it, like Enter. Clicking an enabled action runs it.
func (m *Model) clickAgents(x, y int) {
	screen := m.agentsScreen(m.height - chromeLines)
	view := m.app.Agents.View()
	for _, hit := range screen.actions {
		if hit.rect.Contains(x, y) && view.Actions[hit.index].Enabled {
			m.dispatch(view.Actions[hit.index].Intent)
			return
		}
	}
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
