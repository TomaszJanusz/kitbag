package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

type footerItem struct {
	name  string
	label text.Key
}

func (m *Model) footerItems() []footerItem {
	if m.editing {
		return []footerItem{{name: keyEnter, label: text.FooterSave}, {name: keyEscape, label: text.FooterCancel}}
	}
	quit := text.FooterHome
	if m.home {
		quit = text.FooterQuit
	}
	items := []footerItem{{name: keyHelp, label: text.FooterHelp}, {name: keyQuit, label: quit}}
	if !m.home {
		items = append([]footerItem{{name: keyEscape, label: text.FooterBack}}, items...)
	}
	return items
}

func (m *Model) footerText() string {
	items := m.footerItems()
	args := make([]text.Message, len(items))
	for i, item := range items {
		args[i] = text.New(item.label)
	}
	key := text.ScreenFooter
	if m.home || m.editing {
		key = text.HomeFooter
	}
	return m.say(text.New(key, args...))
}

func (m *Model) footerMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if msg.Y != m.height-1 || msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil, false
	}
	line := m.footerText()
	if m.home {
		line = ansi.Strip(m.homeFooter())
	}
	for _, item := range m.footerItems() {
		label := m.say(text.New(item.label))
		before, _, found := strings.Cut(line, label)
		rect := Rect{X: ansi.StringWidth(before), Y: m.height - 1, Width: ansi.StringWidth(label), Height: 1}
		if found && rect.Contains(msg.X, msg.Y) {
			key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(item.name)}
			if item.name == keyEscape {
				key = tea.KeyMsg{Type: tea.KeyEsc}
			}
			if item.name == keyEnter {
				key = tea.KeyMsg{Type: tea.KeyEnter}
			}
			return m.key(key), true
		}
	}
	return nil, false
}
