package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

const (
	homeMenuWidth     = 38
	pixelMinWidth     = 80
	pixelMinHeight    = 24
	homeCompactHeight = 16
)

type homeLayout struct {
	lines  []string
	menu   []Rect
	status Rect
}

func (m *Model) homeLayout() homeLayout {
	view := m.app.Home(m.version)
	lines := m.logo()
	compact := m.height < homeCompactHeight
	if !compact {
		lines = append(lines, "")
	}
	statusRow := len(lines)
	prefix := indent
	if m.menu == -1 {
		prefix = marker
	}
	lines = append(lines, m.centerLine(m.homeStyle(view.Level).Render(prefix+m.say(view.Headline))))
	if !compact {
		if secondary := m.say(view.Secondary); secondary != "" {
			lines = append(lines, m.centerLine(m.homeStyle(view.SecondaryLevel).Render(secondary)))
		}
		lines = append(lines, "")
	}
	width := min(homeMenuWidth, m.width-dialogMargin)
	left := (m.width - width) / centerDivisor
	border := "+" + strings.Repeat("-", width-borderEdges) + "+"
	if !compact {
		lines = append(lines, strings.Repeat(" ", left)+m.styles.faint.Render(border))
	}
	hits := make([]Rect, len(view.Items))
	for i, item := range view.Items {
		row := m.homeMenuRow(item, i == m.menu, width-dialogMargin)
		hits[i] = Rect{X: left + borderEdges, Y: len(lines), Width: width - dialogMargin, Height: 1}
		lines = append(lines, strings.Repeat(" ", left)+m.styles.faint.Render("| ")+row+m.styles.faint.Render(" |"))
	}
	if !compact {
		lines = append(lines, strings.Repeat(" ", left)+m.styles.faint.Render(border))
	}
	top := max((m.height-1-len(lines))/centerDivisor, 0)
	for i := range hits {
		hits[i].Y += top
	}
	status := Rect{X: 0, Y: statusRow + top, Width: m.width, Height: 1}
	return homeLayout{lines: append(make([]string, top), lines...), menu: hits, status: status}
}

func (m *Model) homeMenuRow(item model.HomeItem, selected bool, width int) string {
	prefix, style := indent, m.styles.tab
	if selected {
		prefix, style = marker, m.styles.heading
	}
	label := prefix + m.say(item.Label)
	value := m.say(item.Value)
	if !item.Enabled {
		value = m.say(text.New(text.HomeUnavailable))
	}
	right := value + "  " + m.say(item.Shortcut)
	return edgeLine(style.Render(label), m.styles.faint.Render(right), width)
}

func (m *Model) homeStyle(level model.Severity) lipgloss.Style {
	switch level {
	case model.Warning:
		return m.styles.warning
	case model.Failure:
		return m.styles.failure
	case model.Info:
		return m.styles.faint
	}
	return m.styles.faint
}

func (m *Model) centerLine(line string) string {
	line = ansi.Truncate(line, max(m.width-dialogMargin, 1), ellipsis)
	return strings.Repeat(" ", max((m.width-ansi.StringWidth(line))/centerDivisor, 0)) + line
}

// The pixels are decorative; the plain brand label always comes from catalog.
func (m *Model) logo() []string {
	if m.styles.mono || m.width < pixelMinWidth || m.height < pixelMinHeight {
		return []string{m.centerLine(m.styles.heading.Render(m.say(text.New(text.HomeBrand))))}
	}
	glyphs := [][]string{
		{"█  █", "█ █ ", "██  ", "█ █ ", "█  █"},
		{" ▀ ", "   ", " █ ", " █ ", " █ "},
		{" █ ", "███", " █ ", " █ ", " ▀▀"},
		{"█   ", "█   ", "███▄", "█  █", "███▀"},
		{"    ", "    ", "▄▀▀█", "█  █", "▀▄▄█"},
		{"    ", "    ", "▄▀▀█", "▀▄▄█", "▄▄▄▀"},
	}
	lines := make([]string, len(glyphs[0]))
	for row := range lines {
		parts := make([]string, len(glyphs))
		for i, glyph := range glyphs {
			parts[i] = glyph[row]
		}
		lines[row] = m.centerLine(m.styles.heading.Render(strings.Join(parts, " ")))
	}
	return lines
}

func (m *Model) homeFooter() string {
	view := m.app.Home(m.version)
	center := m.footerText()
	middle := max((m.width-ansi.StringWidth(center))/centerDivisor, 0)
	leftWidth := max(middle-1, 0)
	branch := ansi.Truncate(m.say(view.Branch), max(leftWidth/centerDivisor, 1), ellipsis)
	path := ansi.Truncate(m.say(view.Repository), max(leftWidth-ansi.StringWidth(branch)-paneGap, 1), ellipsis)
	left := cellLine(m.say(text.New(text.HomeRepository, text.Plain(path), text.Plain(branch))), leftWidth)
	rightWidth := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(center)-1, 0)
	right := ansi.Truncate(m.say(view.Version), rightWidth, ellipsis)
	return m.styles.faint.Render(left + " " + center + strings.Repeat(" ", max(rightWidth-ansi.StringWidth(right), 0)) + right)
}
