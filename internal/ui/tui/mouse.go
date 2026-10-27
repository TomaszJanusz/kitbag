package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// mouse turns a mouse event into the same intents and view state changes as
// the keyboard: screens open, lists select and the wheel scrolls. The overlay
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
	if cmd, consumed := m.footerMouse(msg); consumed {
		return cmd
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
	if m.home {
		m.clickHome(x, y)
		return
	}
	if y == 0 && x < ansi.StringWidth(m.say(text.New(text.ScreenBreadcrumb, m.screenTitle()))) {
		m.back()
		return
	}
	if m.clickAction(x, y) {
		return
	}
	switch m.tab {
	case driftTab:
		m.clickDrift(x, y)
	case settingsTab:
		m.clickSetting(x, y)
	case agentsTab:
		m.clickAgents(x, y)
	case statusTab, logTab, tabCount:
	}
}

// clickDrift selects the clicked target; clicking the selected one again
// opens its details, like Enter.
func (m *Model) clickDrift(x, y int) {
	if m.detail {
		return
	}
	screen := m.driftScreen(m.bodyHeight())
	if !screen.panes.Master.Contains(x, y-(chromeLines-1)) {
		return
	}
	layout := screen.list
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
	screen := m.settingsScreen(m.bodyHeight())
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
	case m.home:
		if direction < 0 {
			m.homeKey(keyUp)
		} else {
			m.homeKey(keyDown)
		}
	case m.tab == statusTab:
		m.scrollStatus(direction)
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
	screen := m.settingsScreen(m.bodyHeight())
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
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && m.clickAction(msg.X, msg.Y) {
		return nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.clickOption(msg.X, msg.Y)
	}
	return nil
}
