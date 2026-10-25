package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// mouse turns a mouse event into the same intents and view state changes as
// the keyboard: tabs switch, lists select and the wheel scrolls. The overlay
// and the settings editor swallow the mouse like they swallow keys, and
// nothing runs before the first size is known.
func (m *Model) mouse(msg tea.MouseMsg) {
	if m.help || m.editing || m.width < MinWidth || m.height < MinHeight {
		return
	}
	switch {
	case msg.Action != tea.MouseActionPress:
	case msg.Button == tea.MouseButtonLeft:
		m.click(msg.X, msg.Y)
	case msg.Button == tea.MouseButtonWheelUp:
		m.wheel(-1)
	case msg.Button == tea.MouseButtonWheelDown:
		m.wheel(1)
	}
}

func (m *Model) click(x, y int) {
	if y == 0 {
		_, spans := m.tabLayout()
		for i, span := range spans {
			if x >= span.start && x < span.end {
				m.tab = tab(i)
			}
		}
		return
	}
	switch m.tab {
	case driftTab:
		m.clickDrift(y)
	case settingsTab:
		m.clickSetting(y)
	case statusTab, logTab, tabCount:
	}
}

// clickDrift selects the clicked target; clicking the selected one again
// opens its details, like Enter.
func (m *Model) clickDrift(y int) {
	if m.detail {
		return
	}
	layout := m.driftLayout(m.height - chromeLines)
	index := layout.first + y - layout.top()
	if y < layout.top() || y-layout.top() >= layout.rows {
		return
	}
	if m.app.Drift.View().Rows[index].Selected {
		m.driftKey(keyEnter)
		return
	}
	m.dispatch(model.SelectDrift{Index: index})
}

// clickSetting selects the clicked setting. Editing stays on Enter so a stray
// click never starts a change.
func (m *Model) clickSetting(y int) {
	layout, _ := m.settingsLayout(m.height - chromeLines)
	if y < layout.top() || y-layout.top() >= layout.rows {
		return
	}
	m.setting = layout.first + y - layout.top()
}

// wheel scrolls the drift list one row or the activity log one entry per
// notch, exactly like the arrow keys.
func (m *Model) wheel(direction int) {
	switch {
	case m.tab == driftTab && !m.detail:
		m.dispatch(model.MoveDrift{Delta: direction})
	case m.tab == logTab:
		m.scrollLog(direction, m.logRows())
	}
}
