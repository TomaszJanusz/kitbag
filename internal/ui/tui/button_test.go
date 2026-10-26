package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func TestButtonProfilesAndStates(t *testing.T) {
	t.Parallel()
	for _, profile := range []Profile{termenv.Ascii, termenv.ANSI, termenv.ANSI256, termenv.TrueColor} {
		m := New(Options{Profile: profile})
		b := button{control: model.Control{Label: text.New(text.DialogOK), Enabled: true}, primary: true, focused: true}
		active := m.buttonView(b, 30)
		if !strings.HasPrefix(active, marker) {
			t.Fatal("focus is invisible")
		}
		if profile == termenv.Ascii {
			if active != "> [ OK ]" || ansiPattern.MatchString(active) {
				t.Fatal(active)
			}
		} else if !strings.Contains(active, "\x1b[") || strings.Contains(active, "[ OK ]") {
			t.Fatal("primary has no filled surface", active)
		}
		b.control.Enabled = false
		disabled := m.buttonView(b, 30)
		if strings.HasPrefix(disabled, marker) || !strings.Contains(disabled, "unavailable") {
			t.Fatal(disabled)
		}
		m.Close()
	}
}

func TestDisabledButtonClickCannotActivate(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 80, 24)
	view := confirmFixture("disabled")
	view.Buttons[0].Enabled = false
	m.ShowDialog(view)
	hit := m.windowLayout(m.windows.top()).hits[0]
	msg := m.windowMouse(tea.MouseMsg{X: hit.rect.X, Y: hit.rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if msg != nil || m.windows.Len() != 1 {
		t.Fatalf("disabled click activated: %#v", msg)
	}
}

func TestDialogScrollingKeepsChromeAndButtons(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 60, 20)
	view := confirmFixture("Scroll test")
	view.Message = text.Plain(strings.Repeat("line of body\n", 40) + "END OF BODY")
	m.ShowDialog(view)
	before := m.View()
	if !strings.Contains(before, "scroll ↑/↓") || !strings.Contains(before, "esc") {
		t.Fatal(before)
	}
	m.windows.Key(keyMsg(keyPageDown))
	if m.windows.top().scroll == 0 {
		t.Fatal("page did not scroll")
	}
	for range 40 {
		m.windowMouse(tea.MouseMsg{X: 30, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	after := m.View()
	for _, fragment := range []string{"END OF BODY", "Scroll test", "esc", "[ Yes ]", "> [ No ]"} {
		if !strings.Contains(after, fragment) {
			t.Fatalf("lost %q:\n%s", fragment, after)
		}
	}
	m.windows.Key(keyMsg(keyEscape))
	if m.windows.Len() != 0 {
		t.Fatal("Esc did not close scrolled dialog")
	}
}
