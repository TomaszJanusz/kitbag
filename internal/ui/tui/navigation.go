package tui

import (
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

const helpDialogID = "terminal.help"

// homeFocusOffset converts the status index (-1) to a cyclic menu index.
const homeFocusOffset = 2

func (m *Model) homeKey(name string) {
	count := len(m.app.Home(m.version).Items) + 1
	switch name {
	case keyUp, keyVimUp, keyPrevious:
		m.menu = (m.menu+count)%count - 1
	case keyDown, keyVimDown, keyNext:
		m.menu = (m.menu+homeFocusOffset)%count - 1
	case keyEnter:
		if m.menu == -1 {
			m.openScreen(statusTab)
		} else {
			m.openHomeAction(m.app.Home(m.version).Items[m.menu])
		}
	default:
		for _, item := range m.app.Home(m.version).Items {
			if name == m.say(item.Shortcut) {
				m.openHomeAction(item)
				return
			}
		}
	}
}

func (m *Model) openHomeAction(item model.HomeItem) {
	if !item.Enabled {
		return
	}
	switch item.Action {
	case model.HomeSync:
		m.dispatch(model.SyncNow{})
	case model.HomeAgents:
		m.openScreen(agentsTab)
	case model.HomeDrift:
		m.openScreen(driftTab)
	case model.HomeSettings:
		m.openScreen(settingsTab)
	case model.HomeActivity:
		m.openScreen(logTab)
	}
}

func (m *Model) openScreen(screen tab) { m.home = false; m.tab = screen; m.detail = false }

func (m *Model) back() {
	if m.detail {
		m.detail = false
		return
	}
	m.home = true
}

func (m *Model) screenTitle() text.Message {
	switch m.tab {
	case statusTab:
		return m.app.Status.View().Title
	case driftTab:
		return m.app.Drift.View().Title
	case settingsTab:
		return m.app.Settings.View().Title
	case logTab:
		return m.app.Log.View().Title
	case agentsTab:
		return m.app.Agents.View().Title
	case tabCount:
		return text.New(text.HomeBrand)
	}
	return text.New(text.HomeBrand)
}

func (m *Model) clickHome(x, y int) {
	layout := m.homeLayout()
	if layout.status.Contains(x, y) {
		m.menu = -1
		m.openScreen(statusTab)
		return
	}
	for i, rect := range layout.menu {
		if rect.Contains(x, y) {
			m.menu = i
			m.openHomeAction(m.app.Home(m.version).Items[i])
			return
		}
	}
}

func (m *Model) showHelp() {
	keys := []text.Key{text.HelpTabs, text.HelpRefresh, text.HelpSync, text.HelpCheckSync, text.HelpMove, text.HelpDetail, text.HelpEdit, text.HelpSave, text.HelpDialogs, text.HelpChoices, text.HelpAdvanced, text.HelpFollow, text.HelpMouse, text.HelpToggle, text.HelpQuit}
	body := make([]string, len(keys))
	for i, key := range keys {
		body[i] = m.say(text.New(key))
	}
	m.help = true
	m.ShowDialog(model.DialogView{
		ID: helpDialogID, Title: text.New(text.HelpTitle), Message: text.Plain(strings.Join(body, "\n\n")), Cancel: model.DismissDialog{ID: helpDialogID}, CloseOnAction: true,
		Buttons: []model.Control{{Intent: model.DismissDialog{ID: helpDialogID}, Label: text.New(text.DialogOK), Enabled: true}},
	})
}
