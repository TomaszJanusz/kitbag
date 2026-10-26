package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

const (
	dialogWidth  = 72
	dialogMargin = 4
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
	}
)

func (m *Model) windowLayout(w *dialogWindow) dialogLayout {
	width := min(dialogWidth, m.width-dialogMargin)
	inner := width - dialogMargin
	content, hits, focused := m.dialogContent(w, inner)
	buttons, buttonHits := m.dialogButtons(w, inner)
	hint := strings.Split(ansi.Wrap(m.say(text.New(text.DialogHint)), inner, ""), "\n")
	header := []string{}
	if w.view.Steps > 0 {
		header = append(header, m.say(text.New(text.DialogStep, number(w.view.Step), number(w.view.Steps))))
	}
	buttonRows := max(m.height-borderEdges-borderEdges-len(header)-len(hint)-1, 1)
	buttons, buttonHits = buttonViewport(buttons, buttonHits, w.focus, buttonRows)
	available := max(m.height-borderEdges-borderEdges-len(header)-len(buttons)-len(hint), 1)
	first := window(focused, len(content), available)
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
	title := " " + m.say(w.view.Title) + " "
	title = ansi.Truncate(title, width-borderEdges, ellipsis)
	border := "+" + title + strings.Repeat("-", max(width-borderEdges-ansi.StringWidth(title), 0)) + "+"
	framed := make([]string, 0, len(lines)+borderEdges)
	framed = append(framed, border)
	for _, line := range lines {
		framed = append(framed, "| "+cellLine(line, inner)+" |")
	}
	framed = append(framed, "+"+strings.Repeat("-", width-borderEdges)+"+")
	for i := range hits {
		hits[i].rect.X += bounds.X + borderEdges
		hits[i].rect.Y += bounds.Y + 1
	}
	return dialogLayout{bounds: bounds, lines: framed, hits: hits}
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

func (m *Model) dialogContent(w *dialogWindow, width int) ([]string, []dialogHit, int) {
	var lines []string
	if message := m.say(w.view.Message); message != "" {
		lines = append(lines, strings.Split(ansi.Wrap(message, width, ""), "\n")...)
	}
	hits := make([]dialogHit, 0, len(w.view.Fields))
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
	return lines, hits, focused
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
		if w.focus == len(w.view.Fields)+i {
			prefix = marker
		}
		label = prefix + label
		size := min(ansi.StringWidth(label), width)
		if x > 0 && x+size > width {
			lines = append(lines, "")
			x = 0
			y++
		}
		hits = append(hits, dialogHit{rect: Rect{X: x, Y: y, Width: size, Height: 1}, index: len(w.view.Fields) + i})
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
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
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
			w.setFocus(hit.index, 1)
			if hit.index >= len(w.inputs) {
				return m.windows.activate(w)
			}
			return nil
		}
	}
	return nil
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
