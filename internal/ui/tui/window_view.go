package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

const (
	dialogWidth     = 72
	dialogWideWidth = 104
	dialogMargin    = 4
)

type (
	dialogHit struct {
		rect  Rect
		index int
	}
	dialogLayout struct {
		lines  []string
		hits   []dialogHit
		bounds Rect
		// page is the number of content lines shown; maxScroll the last top.
		page, maxScroll int
	}
)

func (m *Model) windowLayout(w *dialogWindow) dialogLayout {
	width := dialogWidth
	if w.view.Wide {
		width = dialogWideWidth
	}
	width = min(width, m.width-dialogMargin)
	inner := width - dialogMargin
	content, hits, focused := m.dialogContent(w, inner)
	buttons, buttonHits := m.dialogButtons(w, inner)
	header := []string{}
	if w.view.Steps > 0 {
		header = append(header, m.say(text.New(text.DialogStep, number(w.view.Step), number(w.view.Steps))))
	}
	hint := m.dialogHint(w, inner, func(hint []string) bool {
		return len(content) > m.height-borderEdges-borderEdges-len(header)-len(buttons)-len(hint)
	})
	buttonRows := max(m.height-borderEdges-borderEdges-len(header)-len(hint)-1, 1)
	buttons, buttonHits = buttonViewport(buttons, buttonHits, w.focus, buttonRows)
	available := max(m.height-borderEdges-borderEdges-len(header)-len(buttons)-len(hint), 1)
	maxScroll := max(len(content)-available, 0)
	first := min(max(w.scroll, 0), maxScroll)
	if w.focus >= 0 && w.focus < w.buttons() {
		first = window(focused, len(content), available)
	}
	visible := min(available, len(content))
	lines := append([]string{}, header...)
	lines = append(lines, content[first:first+visible]...)
	hits = visibleDialogHits(hits, first, visible)
	for i := range hits {
		hits[i].rect.Y += len(header)
	}
	for _, hit := range buttonHits {
		hit.rect.Y += len(lines)
		hits = append(hits, hit)
	}
	lines = append(lines, buttons...)
	lines = append(lines, hint...)
	height := len(lines) + borderEdges
	bounds := Rect{X: (m.width - width) / centerDivisor, Y: (m.height - height) / centerDivisor, Width: width, Height: height}
	framed := m.frame(w.view.Title, lines, width)
	for i := range hits {
		hits[i].rect.X += bounds.X + borderEdges
		hits[i].rect.Y += bounds.Y + 1
	}
	return dialogLayout{bounds: bounds, lines: framed, hits: hits, page: available, maxScroll: maxScroll}
}

// dialogHint is the key hint; options add their keys and content that
// overflows (judged by overflows with the hint's lines) adds scrolling.
func (m *Model) dialogHint(w *dialogWindow, width int, overflows func([]string) bool) []string {
	hint := m.say(text.New(text.DialogHint))
	if len(w.view.Options) > 0 {
		hint += " · " + m.say(text.New(text.DialogChoiceHint))
	}
	lines := strings.Split(ansi.Wrap(hint, width, ""), "\n")
	if !overflows(lines) {
		return lines
	}
	return strings.Split(ansi.Wrap(hint+" · "+m.say(text.New(text.DialogScrollHint)), width, ""), "\n")
}

// frame draws the titled border around lines.
func (m *Model) frame(title text.Message, lines []string, width int) []string {
	label := ansi.Truncate(" "+m.say(title)+" ", width-borderEdges, ellipsis)
	framed := make([]string, 0, len(lines)+borderEdges)
	framed = append(framed, "+"+label+strings.Repeat("-", max(width-borderEdges-ansi.StringWidth(label), 0))+"+")
	for _, line := range lines {
		framed = append(framed, "| "+cellLine(line, width-dialogMargin)+" |")
	}
	return append(framed, "+"+strings.Repeat("-", width-borderEdges)+"+")
}

func visibleDialogHits(hits []dialogHit, first, rows int) []dialogHit {
	visible := make([]dialogHit, 0, len(hits))
	for _, hit := range hits {
		if hit.rect.Y >= first && hit.rect.Y < first+rows {
			hit.rect.Y -= first
			visible = append(visible, hit)
		}
	}
	return visible
}

// dialogContent lays out the message, fields, body and options; focused is
// the content line of the focused field or option.
func (m *Model) dialogContent(w *dialogWindow, width int) ([]string, []dialogHit, int) {
	local := *m
	local.width = width
	var lines []string
	if message := m.say(w.view.Message); message != "" {
		lines = append(lines, strings.Split(ansi.Wrap(message, width, ""), "\n")...)
	}
	hits := make([]dialogHit, 0, len(w.view.Fields)+len(w.view.Options))
	focused := 0
	for i, field := range w.view.Fields {
		label := ansi.Truncate(m.say(field.Label), max(width/2-inputMargin, 1), ellipsis) + ": "
		input := w.inputs[i]
		input.Prompt = indent
		if i == w.focus {
			input.Prompt = marker
		}
		input.Width = max(width-ansi.StringWidth(label)-inputMargin, 1)
		input.SetCursor(input.Position())
		input.TextStyle = m.styles.tab
		input.PromptStyle = m.styles.tab
		input.Cursor.Style = m.styles.selected
		input.Cursor.TextStyle = m.styles.tab
		if i == w.focus {
			focused = len(lines)
		}
		hits = append(hits, dialogHit{rect: Rect{Y: len(lines), Width: width, Height: 1}, index: i})
		lines = append(lines, label+input.View())
		if hint := m.say(field.Hint); hint != "" {
			lines = append(lines, strings.Split(ansi.Wrap(hint, width, ""), "\n")...)
		}
	}
	for _, line := range w.view.Body {
		lines = append(lines, local.dialogLine(line)...)
	}
	for i, option := range w.view.Options {
		index := len(w.view.Fields) + i
		if index == w.focus {
			focused = len(lines)
		}
		hits = append(hits, dialogHit{rect: Rect{Y: len(lines), Width: width, Height: 1}, index: index})
		lines = append(lines, m.dialogOption(option, index == w.focus))
	}
	return lines, hits, focused
}

// Text markers keep a body line's tone without colour.
const (
	markSuccess = "✓ "
	markWarning = "! "
	markProblem = "✗ "
)

// dialogLine wraps a body paragraph in its tone; code stays verbatim.
func (m *Model) dialogLine(line model.DialogLine) []string {
	content := m.say(line.Text)
	switch line.Tone {
	case model.ToneHeading:
		return m.wrap(m.styles.heading, content)
	case model.ToneHint:
		return m.wrap(m.styles.faint, content)
	case model.ToneSuccess:
		return m.wrap(m.styles.success, markSuccess+content)
	case model.ToneWarning:
		return m.wrap(m.styles.warning, markWarning+content)
	case model.ToneProblem:
		return m.wrap(m.styles.failure, markProblem+content)
	case model.ToneCode:
		return []string{m.codeStyle(content).Render(ansi.Truncate(indent+content, m.width, ellipsis))}
	case model.ToneNormal:
	}
	return m.wrap(m.styles.tab, content)
}

// codeStyle colours added and removed diff lines; the +/- stays as text.
func (m *Model) codeStyle(line string) lipgloss.Style {
	switch {
	case strings.HasPrefix(line, "+"):
		return m.styles.success
	case strings.HasPrefix(line, "-"):
		return m.styles.failure
	}
	return m.styles.tab
}

// dialogOption is one choice: the focus marker, (*) for the chosen option,
// the label and a faint detail.
func (m *Model) dialogOption(option model.DialogOption, focused bool) string {
	prefix, style, choice := indent, m.styles.tab, "( ) "
	if focused {
		prefix, style = marker, m.styles.selected
	}
	if option.Selected {
		choice = "(*) "
	}
	line := style.Render(prefix + choice + m.say(option.Label))
	if detail := m.say(option.Detail); detail != "" {
		line += m.styles.faint.Render(" · " + detail)
	}
	return line
}

func (m *Model) dialogButtons(w *dialogWindow, width int) ([]string, []dialogHit) {
	lines := []string{""}
	hits := make([]dialogHit, 0, len(w.view.Buttons))
	x, y := 0, 0
	for i, button := range w.view.Buttons {
		label := "[ " + m.say(button.Label) + " ]"
		if !button.Enabled {
			label = m.say(text.New(text.ControlDisabled, text.Plain(label)))
		}
		prefix := indent
		if w.focus == w.buttons()+i {
			prefix = marker
		}
		label = prefix + label
		size := min(ansi.StringWidth(label), width)
		if x > 0 && x+size > width {
			lines = append(lines, "")
			x = 0
			y++
		}
		hits = append(hits, dialogHit{rect: Rect{X: x, Y: y, Width: size, Height: 1}, index: w.buttons() + i})
		lines[y] += ansi.Truncate(label, width, ellipsis)
		x += size
	}
	return lines, hits
}

func (m *Model) overlayWindows(screen []string) []string {
	for _, w := range m.windows.windows {
		layout := m.windowLayout(w)
		for i, line := range layout.lines {
			y := layout.bounds.Y + i
			base := cellLine(screen[y], m.width)
			screen[y] = ansi.Cut(base, 0, layout.bounds.X) + line + ansi.Cut(base, layout.bounds.X+layout.bounds.Width, m.width)
		}
	}
	return screen
}

func (m *Model) windowMouse(msg tea.MouseMsg) model.Msg {
	if msg.Action != tea.MouseActionPress || m.windowWheel(msg.Button) || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	w := m.windows.top()
	layout := m.windowLayout(w)
	if !layout.bounds.Contains(msg.X, msg.Y) {
		m.windows.Pop()
		return w.view.Cancel
	}
	for _, hit := range layout.hits {
		if hit.rect.Contains(msg.X, msg.Y) && w.enabled(hit.index) {
			w.moved = true
			w.setFocus(hit.index, 1)
			switch {
			case hit.index >= w.buttons():
				return m.windows.activate(w)
			case w.option() >= 0:
				return w.view.Options[w.option()].Intent
			}
			return nil
		}
	}
	return nil
}

// windowWheel scrolls the top window one line per notch and reports
// whether button was the wheel.
func (m *Model) windowWheel(button tea.MouseButton) bool {
	if button != tea.MouseButtonWheelUp && button != tea.MouseButtonWheelDown {
		return false
	}
	lines := 1
	if button == tea.MouseButtonWheelUp {
		lines = -1
	}
	m.scrollWindow(lines)
	return true
}

// scrollWindow moves the top window's content by lines (the wheel) or, for
// PgUp/PgDn, by pages; a focused field or option keeps itself visible.
func (m *Model) scrollWindow(lines int) {
	w := m.windows.top()
	if w == nil {
		return
	}
	layout := m.windowLayout(w)
	w.scroll = min(max(min(max(w.scroll, 0), layout.maxScroll)+lines, 0), layout.maxScroll)
}

// windowPage scrolls the top window by one page.
func (m *Model) windowPage(direction int) {
	w := m.windows.top()
	if w == nil {
		return
	}
	m.scrollWindow(direction * max(m.windowLayout(w).page-1, 1))
}

// ShowDialog projects a model-owned window. Repeated IDs replace the current
// step and preserve focus instead of pushing duplicate overlays.
func (m *Model) ShowDialog(view model.DialogView) { m.windows.Replace(view) }

// HideDialog removes a window after its owning model closes it. Unlike user
// cancellation this sends no intent, and windows above it retain their focus.
func (m *Model) HideDialog(id string) { m.windows.Remove(id) }

func buttonViewport(lines []string, hits []dialogHit, focus, rows int) ([]string, []dialogHit) {
	selected := 0
	for _, hit := range hits {
		if hit.index == focus {
			selected = hit.rect.Y
		}
	}
	first := window(selected, len(lines), rows)
	visible := min(rows, len(lines))
	return lines[first : first+visible], visibleDialogHits(hits, first, visible)
}
