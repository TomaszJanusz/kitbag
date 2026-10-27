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
	header := []string{edgeLine(m.say(w.view.Title), m.say(text.New(text.DialogEscape)), inner)}
	if w.view.Steps > 0 {
		header = append(header, m.say(text.New(text.DialogStep, number(w.view.Step), number(w.view.Steps))))
	}
	header = append(header, "")
	buttons, buttonHits := m.dialogButtons(w, inner)
	// Reserve a body row and a scroll-hint row even at the minimum size.
	buttonRows := max(m.height-borderEdges-len(header)-borderEdges, 1)
	buttons, buttonHits = buttonViewport(buttons, buttonHits, w.focus, buttonRows)
	available := max(m.height-borderEdges-len(header)-len(buttons)-1, 1)
	first, visible := w.viewport(len(content), focused, available)
	lines := append([]string{}, header...)
	lines = append(lines, content[first:first+visible]...)
	hits = visibleDialogHits(hits, first, visible)
	for i := range hits {
		hits[i].rect.Y += len(header)
	}
	hint := ""
	if w.maxScroll > 0 {
		hint = m.say(text.New(text.DialogScroll))
	}
	lines = append(lines, m.styles.faint.Render(hint))
	for _, hit := range buttonHits {
		hit.rect.Y += len(lines)
		hits = append(hits, hit)
	}
	lines = append(lines, buttons...)
	bounds := Rect{X: (m.width - width) / centerDivisor, Y: (m.height - len(lines) - borderEdges) / centerDivisor, Width: width, Height: len(lines) + borderEdges}
	framed := m.dialogFrame(lines, width, inner)
	for i := range hits {
		hits[i].rect.X += bounds.X + borderEdges
		hits[i].rect.Y += bounds.Y + 1
	}
	return dialogLayout{bounds: bounds, lines: framed, hits: hits, page: available, maxScroll: w.maxScroll}
}

func edgeLine(left, right string, width int) string {
	right = ansi.Truncate(right, width, ellipsis)
	left = ansi.Truncate(left, max(width-ansi.StringWidth(right)-1, 0), ellipsis)
	return left + strings.Repeat(" ", max(width-ansi.StringWidth(left)-ansi.StringWidth(right), 0)) + right
}

func (w *dialogWindow) viewport(count, focused, rows int) (int, int) {
	w.rows, w.maxScroll = rows, max(count-rows, 0)
	if w.followFocus && w.focus >= 0 && w.focus < w.buttons() {
		if focused < w.scroll {
			w.scroll = focused
		}
		if focused >= w.scroll+rows {
			w.scroll = focused - rows + 1
		}
	}
	w.followFocus = false
	w.scroll = min(max(w.scroll, 0), w.maxScroll)
	return w.scroll, min(rows, count)
}

func (m *Model) dialogFrame(lines []string, width, inner int) []string {
	top, bottom := strings.Repeat(" ", width), strings.Repeat(" ", width)
	if m.styles.mono {
		top = "+" + strings.Repeat("-", width-borderEdges) + "+"
		bottom = top
	}
	framed := make([]string, 0, len(lines)+borderEdges)
	framed = append(framed, m.styles.panel.Render(top))
	for _, line := range lines {
		left, right := "  ", "  "
		if m.styles.mono {
			left, right = "| ", " |"
		}
		framed = append(framed, m.panelLine(left+cellLine(line, inner)+right))
	}
	return append(framed, m.styles.panel.Render(bottom))
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
		lines = append(lines, styledLines(m.styles.faint, message, width)...)
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
			lines = append(lines, styledLines(m.styles.faint, hint, width)...)
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
	return m.wrap(m.styles.faint, content)
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
	for i, control := range w.view.Buttons {
		label := m.buttonView(button{control: control, focused: w.focus == w.buttons()+i, primary: primaryButton(w.view, i)}, width)
		size := ansi.StringWidth(label)
		if x > 0 && x+size+1 > width {
			lines = append(lines, "")
			x = 0
			y++
		}
		hits = append(hits, dialogHit{rect: Rect{X: x, Y: y, Width: size, Height: 1}, index: w.buttons() + i})
		if x > 0 {
			lines[y] += " "
			hits[len(hits)-1].rect.X++
			x++
		}
		lines[y] += label
		x += size
	}
	for row, line := range lines {
		offset := max(width-ansi.StringWidth(line), 0)
		lines[row] = strings.Repeat(" ", offset) + line
		for i := range hits {
			if hits[i].rect.Y == row {
				hits[i].rect.X += offset
			}
		}
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
	w := m.windows.top()
	layout := m.windowLayout(w)
	if msg.Action != tea.MouseActionPress {
		return nil
	}
	if scrollWindowMouse(w, layout.bounds, msg) {
		return nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
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

// scrollWindow moves the top window's content by lines (the wheel) or, for
// PgUp/PgDn, by pages; a focused field or option keeps itself visible.
func (m *Model) scrollWindow(lines int) {
	w := m.windows.top()
	if w == nil {
		return
	}
	m.windowLayout(w)
	w.scrollBody(lines)
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

func styledLines(style lipgloss.Style, value string, width int) []string {
	lines := strings.Split(ansi.Wrap(value, width, ""), "\n")
	for i := range lines {
		lines[i] = style.Render(lines[i])
	}
	return lines
}

// Restore the panel surface after nested text/button ANSI resets, including
// padding to the right of a muted body line.
func (m *Model) panelLine(line string) string {
	if m.styles.mono {
		return line
	}
	sample := m.styles.panel.Render("x")
	prefix, _, _ := strings.Cut(sample, "x")
	return prefix + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix) + "\x1b[0m"
}

func scrollWindowMouse(w *dialogWindow, bounds Rect, msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return false
	}
	delta := 1
	if msg.Button == tea.MouseButtonWheelUp {
		delta = -1
	}
	if bounds.Contains(msg.X, msg.Y) {
		w.scrollBody(delta)
	}
	return true
}
