package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// Layout: one tab bar line, a blank separator and one footer line.
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
	body := m.height - chromeLines
	var lines []string
	switch {
	case m.help:
		lines = m.helpView()
	case m.tab == driftTab:
		lines = m.driftView(body)
	case m.tab == settingsTab:
		lines = m.settingsView(body)
	case m.tab == logTab:
		lines = m.logView(body)
	default:
		lines = m.statusView()
	}
	screen := make([]string, 0, m.height)
	screen = append(screen, m.tabBar(), "")
	screen = append(screen, pad(m.fit(lines, body), body)...)
	screen = append(screen, m.styles.faint.Render(m.say(text.New(text.TerminalFooter))))
	return strings.Join(m.fit(screen, m.height), "\n")
}

func (m *Model) say(message text.Message) string { return m.catalog.Render(message) }

func number(n int) text.Message { return text.Plain(strconv.Itoa(n)) }

// tabBar names every tab; a narrow terminal shows only the active title.
func (m *Model) tabBar() string {
	titles := []text.Message{m.app.Status.View().Title, m.app.Drift.View().Title, m.app.Settings.View().Title, m.app.Log.View().Title}
	full := make([]string, 0, len(titles))
	compact := make([]string, 0, len(titles))
	for i, title := range titles {
		label := strconv.Itoa(i+1) + " " + m.say(title)
		if tab(i) == m.tab {
			active := m.styles.active.Render("[" + label + "]")
			full, compact = append(full, active), append(compact, active)
			continue
		}
		full = append(full, m.styles.tab.Render(" "+label+" "))
		compact = append(compact, m.styles.tab.Render(strconv.Itoa(i+1)))
	}
	if bar := strings.Join(full, " "); ansi.StringWidth(bar) <= m.width {
		return bar
	}
	return strings.Join(compact, " ")
}

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

func (m *Model) problem(problem *text.Message) []string {
	if problem == nil {
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

// controls renders actions with their keys; disabled ones say so in text.
func (m *Model) controls(controls []model.Control) string {
	parts := make([]string, 0, len(controls))
	for _, c := range controls {
		label := text.New(text.ControlHint, text.Plain(intentName(c.Intent)), c.Label)
		if !c.Enabled {
			parts = append(parts, m.styles.faint.Render(m.say(text.New(text.ControlDisabled, label))))
			continue
		}
		parts = append(parts, m.say(label))
	}
	return strings.Join(parts, "  ")
}

func intentName(intent model.Msg) string {
	switch intent.(type) {
	case model.SyncNow:
		return keySync
	case model.Refresh:
		return keyRefresh
	case model.SaveSetting:
		return keySave
	case model.UnsetSetting:
		return keyUnset
	case model.RevertSetting:
		return keyEscape
	default:
		return ""
	}
}

func (m *Model) helpView() []string {
	keys := []text.Key{
		text.HelpTabs, text.HelpRefresh, text.HelpSync, text.HelpMove, text.HelpDetail, text.HelpEdit,
		text.HelpSave, text.HelpChoices, text.HelpFollow, text.HelpToggle, text.HelpQuit,
	}
	lines := make([]string, 0, len(keys)+1)
	lines = append(lines, m.styles.heading.Render(m.say(text.New(text.HelpTitle))))
	lines = append(lines, "")
	for _, key := range keys {
		lines = append(lines, m.wrap(m.styles.tab, indent+m.say(text.New(key)))...)
	}
	return lines
}
