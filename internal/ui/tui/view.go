package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Layout: one breadcrumb line, a blank separator and one footer line.
const (
	chromeLines = 3
	ellipsis    = "…"
	bullet      = "- "
	indent      = "  "
	marker      = "> "
)

// View renders the whole screen; every line fits the terminal width.
func (m *Model) View() string {
	if m.quitting || m.width == 0 {
		return ""
	}
	if m.width < MinWidth || m.height < MinHeight {
		return strings.Join(m.fit(m.wrap(m.styles.warning, m.say(text.New(text.TerminalTooSmall,
			number(MinWidth), number(MinHeight), number(m.width), number(m.height)))), m.height), "\n")
	}
	body := m.bodyHeight()
	lines := m.screenBody(body)
	screen := make([]string, 0, m.height)
	if m.home {
		screen = pad(m.fit(lines, m.height-1), m.height-1)
		screen = append(screen, m.homeFooter())
	} else {
		screen = append(screen, m.styles.faint.Render(m.say(text.New(text.ScreenBreadcrumb, m.screenTitle()))), "")
		screen = append(screen, pad(m.fit(lines, body), body)...)
		screen = append(screen, m.actionLayout().lines...)
		screen = append(screen, m.styles.faint.Render(m.footerText()))
	}
	for i := range screen {
		if m.windows.Len() > 0 {
			screen[i] = ""
		}
		screen[i] = m.surfaceLine(screen[i])
	}
	return strings.Join(m.overlayWindows(m.fit(screen, m.height)), "\n")
}

func (m *Model) say(message text.Message) string { return m.catalog.Render(message) }

func number(n int) text.Message { return text.Plain(strconv.Itoa(n)) }

// wrap word-wraps plain text to the width and styles each resulting line.
func (m *Model) wrap(style lipgloss.Style, plain string) []string {
	wrapped := strings.Split(ansi.Wrap(plain, m.width, ""), "\n")
	for i, line := range wrapped {
		wrapped[i] = style.Render(line)
	}
	return wrapped
}

// fit truncates lines to the width and the slice to height.
func (m *Model) fit(lines []string, height int) []string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, ellipsis)
	}
	return lines
}

func pad(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// window returns the first index that keeps selected visible among rows.
func window(selected, count, rows int) int {
	if rows <= 0 || count <= rows {
		return 0
	}
	return min(max(selected-rows/2, 0), count-rows)
}

// problem renders a view's problem unless the headline already says it.
func (m *Model) problem(problem *text.Message, headline text.Message) []string {
	if problem == nil || m.say(*problem) == m.say(headline) {
		return nil
	}
	return m.wrap(m.styles.failure, m.say(*problem))
}

func (m *Model) warnings(warnings []text.Message) []string {
	if len(warnings) == 0 {
		return nil
	}
	lines := []string{"", m.styles.heading.Render(m.say(text.New(text.TerminalWarnings)))}
	for _, warning := range warnings {
		lines = append(lines, m.wrap(m.styles.warning, bullet+m.say(warning))...)
	}
	return lines
}

func (m *Model) screenBody(body int) []string {
	var lines []string
	switch {
	case m.home:
		lines = m.homeLayout().lines
	case m.tab == driftTab:
		lines = m.driftScreen(body).lines
	case m.tab == settingsTab:
		lines = m.settingsView(body)
	case m.tab == logTab:
		lines = m.logView(body)
	case m.tab == agentsTab:
		lines = m.agentsScreen(body).lines
	default:
		lines = m.statusView()
		m.statusTop = min(m.statusTop, max(len(lines)-body, 0))
		lines = lines[m.statusTop:]
	}
	return lines
}
