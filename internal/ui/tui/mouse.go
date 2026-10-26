package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// mouse turns a mouse event into the same intents and view state changes as
// the keyboard: tabs switch, lists select and the wheel scrolls. The overlay
// swallows the mouse; the settings editor accepts only option clicks.
// Nothing runs before the first size is known.
func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if m.mouseUnavailable() {
		return nil
	}
	if m.outsideScreen(msg.X, msg.Y) {
		return nil
	}
	if m.windows.Len() > 0 {
		m.dispatch(m.windowMouse(msg))
		return nil
	}
	if m.editing {
		return m.editorMouse(msg)
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
	return nil
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
		m.clickSetting(x, y)
	case agentsTab:
		m.clickAgents(x, y)
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
func (m *Model) clickSetting(x, y int) {
	screen := m.settingsScreen(m.height - chromeLines)
	if !screen.panes.Master.Contains(x, y-(chromeLines-1)) {
		return
	}
	layout := screen.list
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
	case m.tab == agentsTab:
		m.dispatch(model.MoveAgent{Delta: direction})
	case m.tab == logTab:
		m.scrollLog(direction, m.logRows())
	}
}

// clickOption uses the same rendered option geometry as the screen.
func (m *Model) clickOption(x, y int) tea.Cmd {
	field := m.field()
	if field.Saving {
		return nil
	}
	screen := m.settingsScreen(m.height - chromeLines)
	for _, hit := range screen.hits {
		if y == hit.y && x >= hit.start && x < hit.end {
			m.dispatch(model.ChooseSetting{Key: field.Path, Index: hit.index})
			return m.syncInput()
		}
	}
	return nil
}

func (m *Model) mouseUnavailable() bool {
	return (m.help && m.windows.Len() == 0) || m.width < MinWidth || m.height < MinHeight
}

func (m *Model) outsideScreen(x, y int) bool { return x < 0 || y < 0 || x >= m.width || y >= m.height }

func (m *Model) editorMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.clickOption(msg.X, msg.Y)
	}
	return nil
}
