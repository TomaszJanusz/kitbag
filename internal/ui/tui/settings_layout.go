package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

type settingsScreen struct {
	lines []string
	hits  []optionHit
	list  listLayout
	panes PaneLayout
}

func (m *Model) settingsView(height int) []string { return m.settingsScreen(height).lines }

func (m *Model) settingsScreen(height int) settingsScreen {
	view := m.app.Settings.View()
	if len(view.Fields) == 0 {
		return settingsScreen{lines: m.wrap(m.styles.heading, m.say(view.Headline))}
	}
	field := m.field()
	panes := MasterDetail(m.width, height, 0, max(height/paneGap, 1))
	local := *m
	local.width = panes.Master.Width
	head := local.wrap(m.styles.heading, m.say(view.Headline))
	head = append(head, local.problem(view.Problem, view.Headline)...)
	head = append(head, m.say(view.Advanced.Label))
	if height < compactHeight {
		head = nil
	}
	// The detail and help panel stay visible even on a short stacked terminal.
	detailWidth := panes.Detail.Width
	inline := m.inlineControl(field, panes.Master.Width)
	detail, hits := m.settingDetail(field, detailWidth, inline)
	if panes.Stacked {
		listHeight := max(height-len(detail)-1, len(head)+borderEdges)
		if height < compactHeight {
			listHeight = borderEdges
		}
		panes = MasterDetail(m.width, height, 0, listHeight)
	}
	detail = m.compactDetail(detail, hits, panes.Detail)
	rows := min(max(panes.Master.Height-len(head)-1, 1), len(view.Fields))
	first := window(m.setting, len(view.Fields), rows)
	list := listLayout{head: head, first: first, rows: rows}
	left := append(append([]string{}, head...), "")
	for i, f := range view.Fields[first : first+rows] {
		selected := first+i == m.setting
		name := settingName(f)
		row := m.listRow(selected, name, m.say(f.Value)+" · "+m.say(f.Source))
		if selected && inline {
			prefix := marker + name + indent
			controls, options := m.settingControl(f, panes.Master.Width-ansi.StringWidth(prefix))
			row = m.styles.selected.Render(prefix) + lineAt(controls, 0)
			hits = offsetHits(options.hits, ansi.StringWidth(prefix), list.top()+i)
		}
		left = append(left, ansi.Truncate(row, panes.Master.Width, ellipsis))
	}
	if !inline {
		hits = offsetHits(hits, panes.Detail.X, chromeLines-1+panes.Detail.Y)
	}
	return settingsScreen{panes: panes, list: list, lines: composePanes(panes, left, detail), hits: hits}
}

func settingName(field model.FieldView) string {
	name := field.Path
	if field.Advanced {
		name = indent + name
	}
	if control(field.Actions, model.RevertSetting{Key: field.Path}).Enabled {
		name += " *"
	}
	return name
}

func (m *Model) inlineControl(field model.FieldView, width int) bool {
	if !m.editing {
		return false
	}
	available := width - ansi.StringWidth(marker+settingName(field)+indent)
	controls, _ := m.settingControl(field, available)
	return len(controls) == 1 && available >= inputMargin && ansi.StringWidth(controls[0]) <= available
}

func (m *Model) settingControl(field model.FieldView, width int) ([]string, optionLayout) {
	lines, options := m.optionsAt(field, max(width, 1))
	if field.TextEntry {
		input := m.input
		input.Width = max(width-inputMargin, 1)
		input.SetCursor(input.Position())
		lines = append(lines, input.View())
	}
	return lines, options
}

func (m *Model) settingDetail(field model.FieldView, width int, inline bool) ([]string, []optionHit) {
	local := *m
	local.width = width
	lines := []string{m.styles.heading.Render(field.Path)}
	var hits []optionHit
	// A control that cannot fit in the list sits beside the field in this pane.
	if m.editing && !inline {
		prefix := field.Path + indent
		controls, options := m.settingControl(field, width-ansi.StringWidth(prefix))
		if len(controls) == 1 && ansi.StringWidth(prefix+controls[0]) <= width {
			lines[0] = m.styles.heading.Render(prefix) + controls[0]
			hits = offsetHits(options.hits, ansi.StringWidth(prefix), 0)
		} else {
			controls, options = m.settingControl(field, width)
			hits = offsetHits(options.hits, 0, len(lines))
			lines = append(lines, controls...)
		}
	}
	lines = append(lines, local.wrap(m.styles.tab, m.say(field.Description))...)
	lines = append(lines, local.wrap(m.styles.tab, m.say(text.New(text.SettingValue, field.Value)))...)
	lines = append(lines, local.wrap(m.styles.tab, m.say(text.New(text.SettingSource, field.Source)))...)
	if field.Override != nil {
		lines = append(lines, local.wrap(m.styles.warning, m.say(*field.Override))...)
	}
	lines = append(lines, local.wrap(m.styles.faint, m.say(field.Default))...)
	lines = append(lines, local.problem(field.Problem, text.Message{})...)
	lines = append(lines, "", m.styles.heading.Render("["+m.say(text.New(text.SettingHelp))+"]"))
	if field.Hint != nil {
		lines = append(lines, local.wrap(m.styles.faint, m.say(*field.Hint))...)
	}
	if m.editing {
		return append(lines, local.editorHint(field)...), hits
	}
	if field.Editor != model.SettingReadOnly {
		lines = append(lines, local.wrap(m.styles.faint, m.say(text.New(text.SettingListHint)))...)
	}

	return lines, hits
}

func offsetHits(hits []optionHit, x, y int) []optionHit {
	result := make([]optionHit, len(hits))
	for i, hit := range hits {
		hit.start += x
		hit.end += x
		hit.y += y
		result[i] = hit
	}
	return result
}

// compactHeight reserves a usable editor and help on the minimum-size screen.
const compactHeight = 12

func (m *Model) compactDetail(lines []string, hits []optionHit, rect Rect) []string {
	if len(lines) <= rect.Height {
		return lines
	}
	help := []string{}
	for i, line := range lines {
		if strings.Contains(line, "["+m.say(text.New(text.SettingHelp))+"]") {
			help = lines[i:]
			break
		}
	}
	keep := 1
	for _, hit := range hits {
		keep = max(keep, hit.y+1)
	}
	if len(help)+keep > rect.Height && m.editing {
		help = []string{"[" + m.say(text.New(text.SettingHelp)) + "] " + m.say(text.New(text.SettingCompactHint))}
	}
	keep = min(max(rect.Height-len(help), keep), len(lines))
	return append(append([]string{}, lines[:keep]...), help...)
}
