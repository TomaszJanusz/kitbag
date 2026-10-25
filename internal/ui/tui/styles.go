package tui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Basic ANSI colours follow the terminal's own palette and theme.
const (
	colourRed    = lipgloss.Color("1")
	colourGreen  = lipgloss.Color("2")
	colourYellow = lipgloss.Color("3")
	colourBlue   = lipgloss.Color("4")
	colourGrey   = lipgloss.Color("8")
)

// styles never depend on colour alone: emphasis, markers and labels carry
// the same information on a monochrome (NO_COLOR) terminal.
type styles struct {
	tab, active, heading, faint, warning, failure, success, selected lipgloss.Style
}

// newStyles uses a private renderer with an explicit profile: no terminal
// queries (such as the background colour) and no shared global state.
func newStyles(profile termenv.Profile) styles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(profile)
	r.SetHasDarkBackground(true)
	return styles{
		tab:      r.NewStyle(),
		active:   r.NewStyle().Bold(true).Reverse(true),
		heading:  r.NewStyle().Bold(true).Foreground(colourBlue),
		faint:    r.NewStyle().Faint(true).Foreground(colourGrey),
		warning:  r.NewStyle().Foreground(colourYellow),
		failure:  r.NewStyle().Bold(true).Foreground(colourRed),
		success:  r.NewStyle().Foreground(colourGreen),
		selected: r.NewStyle().Bold(true),
	}
}
