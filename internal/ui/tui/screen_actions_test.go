package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestSettingsRemoveButtonRequiresConfirmation(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 80, 24)
	var remove actionHit
	for _, hit := range m.actionLayout().hits {
		if _, ok := hit.control.Intent.(model.RequestUnsetSetting); ok {
			remove = hit
		}
	}
	m.mouse(tea.MouseMsg{X: remove.rect.X, Y: remove.rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.windows.Len() != 1 || !strings.Contains(m.View(), "Remove stored setting?") {
		t.Fatal("button bypassed confirmation")
	}
	if len(m.queue.pending) != 0 {
		t.Fatal("remove wrote before confirmation")
	}
}

func TestActivityButtonAndStatusScroll(t *testing.T) {
	t.Parallel()
	m := homeProjection(t, 60, 20)
	m.openScreen(logTab)
	hit := m.actionLayout().hits[0]
	m.mouse(tea.MouseMsg{X: hit.rect.X, Y: hit.rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.follow {
		t.Fatal("pause click did not act")
	}
	m.openScreen(statusTab)
	m.width, m.height = 40, 10
	m.statusKey(keyPageDown)
	if m.statusTop == 0 {
		t.Fatal("status cannot scroll to hidden details")
	}
}

func TestHelpScrollAndOKButton(t *testing.T) {
	t.Parallel()
	s := startHome(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press(keyHelp)
	s.waitFor("Keyboard", "esc", "scroll ↑/↓", "> [ OK ]")
	s.press(keyPageDown)
	s.waitFor("Ctrl-C: quit")
	s.press(keyEnter)
	s.waitFor("? help · q quit")
	if screen := s.final(); s.m.windows.Len() != 0 || s.m.help || !s.m.home {
		t.Fatal(screen)
	}
}

func TestFooterMouseHelpQuitAndBack(t *testing.T) {
	t.Parallel()
	m := homeProjection(t, 80, 24)
	line := m.homeFooter()
	before, _, _ := strings.Cut(line, "? help")
	help := ansi.StringWidth(before)
	m.mouse(tea.MouseMsg{X: help, Y: 23, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.help || m.windows.Len() != 1 {
		t.Fatal("help hint is not clickable")
	}
	m.key(keyMsg(keyEscape))
	m.openScreen(settingsTab)
	m.mouse(tea.MouseMsg{X: 0, Y: 23, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.home {
		t.Fatal("back hint did not return home")
	}
	before, _, _ = strings.Cut(m.homeFooter(), "q quit")
	quit := ansi.StringWidth(before)
	cmd := m.mouse(tea.MouseMsg{X: quit, Y: 23, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || !m.quitting {
		t.Fatal("quit hint did not quit")
	}
}
