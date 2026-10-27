package tui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Semantic errors retain the terminal palette; surfaces use explicit fallbacks.
const (
	colourRed   = lipgloss.Color("1")
	colourGreen = lipgloss.Color("2")
)

// styles never depend on colour alone: emphasis, markers and labels carry
// the same information on a monochrome (NO_COLOR) terminal.
type styles struct {
	tab, active, heading, faint, warning, failure, success, selected lipgloss.Style
	primary, secondary, panel, screen                                lipgloss.Style
	logo                                                             [logoShades][logoShades]lipgloss.Style
	mono                                                             bool
}

// newStyles uses a private renderer with an explicit profile: no terminal
// queries (such as the background colour) and no shared global state.
func newStyles(profile termenv.Profile) styles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(profile)
	r.SetHasDarkBackground(true)
	accent := lipgloss.CompleteColor{TrueColor: "#e9b872", ANSI256: "179", ANSI: "3"}
	background := lipgloss.CompleteColor{TrueColor: "#181818", ANSI256: "234", ANSI: "0"}
	panel := lipgloss.CompleteColor{TrueColor: "#242424", ANSI256: "235", ANSI: "0"}
	foreground := lipgloss.CompleteColor{TrueColor: "#dedbd4", ANSI256: "253", ANSI: "7"}
	muted := lipgloss.CompleteColor{TrueColor: "#99958e", ANSI256: "246", ANSI: "8"}
	shades := [logoShades]lipgloss.CompleteColor{
		background, accent,
		{TrueColor: "#8e6338", ANSI256: "95", ANSI: "8"},
		{TrueColor: "#50391f", ANSI256: "58", ANSI: "8"},
	}
	var logo [logoShades][logoShades]lipgloss.Style
	for fg := range logo {
		for bg := range logo[fg] {
			logo[fg][bg] = r.NewStyle().Foreground(shades[fg]).Background(shades[bg])
		}
	}
	return styles{
		logo:      logo,
		mono:      profile == termenv.Ascii,
		primary:   r.NewStyle().Background(accent).Foreground(background).Bold(true),
		secondary: r.NewStyle().Foreground(muted),
		panel:     r.NewStyle().Background(panel).Foreground(foreground),
		screen:    r.NewStyle().Background(background).Foreground(foreground),
		tab:       r.NewStyle(),
		active:    r.NewStyle().Bold(true).Reverse(true),
		heading:   r.NewStyle().Bold(true).Foreground(accent),
		faint:     r.NewStyle().Foreground(muted),
		warning:   r.NewStyle().Foreground(accent),
		failure:   r.NewStyle().Bold(true).Foreground(colourRed),
		success:   r.NewStyle().Foreground(colourGreen),
		selected:  r.NewStyle().Bold(true),
	}
}
