package tui

import (
	"slices"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// Key names as reported by Bubble Tea.
const (
	keyQuit     = "q"
	keyCtrlC    = "ctrl+c"
	keyHelp     = "?"
	keyNext     = "tab"
	keyPrevious = "shift+tab"
	keyRefresh  = "r"
	keySync     = "s"
	keyEnter    = "enter"
	keyEscape   = "esc"
	keyUp       = "up"
	keyDown     = "down"
	keyVimUp    = "k"
	keyVimDown  = "j"
	keyLeft     = "left"
	keyRight    = "right"
	keyPageUp   = "pgup"
	keyPageDown = "pgdown"
	keySave     = "w"
	keyUnset    = "u"
	keyFollow   = "f"
)

// inputMargin leaves room for the prompt and cursor of the settings editor.
const inputMargin = 4

func (m *Model) key(msg tea.KeyMsg) tea.Cmd {
	name := msg.String()
	switch {
	case name == keyCtrlC:
		return m.quit()
	case m.editing:
		return m.editKey(msg)
	case m.help:
		return m.helpKey(name)
	}
	if m.globalKey(name) {
		return m.quitCmd()
	}
	switch m.tab {
	case driftTab:
		m.driftKey(name)
	case settingsTab:
		return m.settingsKey(name)
	case logTab:
		m.logKey(name)
	case statusTab, tabCount:
	}
	return nil
}

// helpKey closes the overlay; other keys are ignored while it is shown.
func (m *Model) helpKey(name string) tea.Cmd {
	switch name {
	case keyQuit:
		return m.quit()
	case keyHelp, keyEscape:
		m.help = false
	}
	return nil
}

// quitCmd returns the quit command once quitting has been requested.
func (m *Model) quitCmd() tea.Cmd {
	if m.quitting {
		return tea.Quit
	}
	return nil
}

// globalKey handles keys shared by every tab and reports whether it consumed
// the key.
func (m *Model) globalKey(name string) bool {
	switch name {
	case keyQuit:
		m.quit()
	case keyHelp:
		m.help = true
	case keyNext:
		m.tab = (m.tab + 1) % tabCount
	case keyPrevious:
		m.tab = (m.tab + tabCount - 1) % tabCount
	case "1", "2", "3", "4":
		m.tab = tab(name[0] - '1')
	case keyRefresh:
		m.dispatch(model.Refresh{})
	case keySync:
		m.dispatch(model.SyncNow{})
	default:
		return false
	}
	return true
}

func (m *Model) driftKey(name string) {
	switch name {
	case keyUp, keyVimUp:
		m.dispatch(model.MoveDrift{Delta: -1})
	case keyDown, keyVimDown:
		m.dispatch(model.MoveDrift{Delta: 1})
	case keyEnter:
		m.detail = m.app.Drift.View().Detail != nil
	case keyEscape:
		m.detail = false
	}
}

func (m *Model) field() model.FieldView {
	fields := m.app.Settings.View().Fields
	m.setting = min(max(m.setting, 0), len(fields)-1)
	return fields[m.setting]
}

func (m *Model) settingsKey(name string) tea.Cmd {
	fields := len(m.app.Settings.View().Fields)
	switch name {
	case keyUp, keyVimUp:
		m.setting = max(m.setting-1, 0)
	case keyDown, keyVimDown:
		m.setting = min(m.setting+1, fields-1)
	case keyEnter:
		return m.startEdit()
	case keySave:
		m.dispatch(model.SaveSetting{Key: m.field().Path})
	case keyUnset:
		m.dispatch(model.UnsetSetting{Key: m.field().Path})
	case keyEscape:
		m.dispatch(model.RevertSetting{Key: m.field().Path})
	}
	return nil
}

func (m *Model) startEdit() tea.Cmd {
	field := m.field()
	if field.Saving {
		return nil
	}
	m.editing = true
	m.input.EchoMode = echoMode(field.Sensitive)
	m.input.SetValue(field.Draft)
	m.input.CursorEnd()
	return m.input.Focus()
}

// editKey forwards keys to the line editor. Enter saves a valid change and
// leaves the editor; an invalid draft stays open with its message. Esc
// discards the draft.
func (m *Model) editKey(msg tea.KeyMsg) tea.Cmd {
	field := m.field()
	switch name := msg.String(); {
	case name == keyEnter:
		m.finishEdit(field)
		return nil
	case name == keyEscape:
		m.dispatch(model.RevertSetting{Key: field.Path})
		m.stopEdit()
		return nil
	case (name == keyLeft || name == keyRight) && len(field.Choices) > 0:
		m.input.SetValue(cycle(field.Choices, m.input.Value(), name == keyRight))
		m.input.CursorEnd()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != field.Draft {
			m.dispatch(model.EditSetting{Key: field.Path, Value: m.input.Value()})
		}
		return cmd
	}
	m.dispatch(model.EditSetting{Key: field.Path, Value: m.input.Value()})
	return nil
}

func (m *Model) finishEdit(field model.FieldView) {
	save, revert := control(field.Actions, model.SaveSetting{Key: field.Path}), control(field.Actions, model.RevertSetting{Key: field.Path})
	switch {
	case save.Enabled:
		m.dispatch(save.Intent)
		m.stopEdit()
	case !revert.Enabled:
		// Nothing changed: leave the editor.
		m.stopEdit()
	}
}

func (m *Model) stopEdit() {
	m.editing = false
	m.input.Blur()
	m.input.SetValue("")
}

func control(controls []model.Control, intent model.Msg) model.Control {
	for _, c := range controls {
		if c.Intent == intent {
			return c
		}
	}
	return model.Control{Intent: intent}
}

// echoMode masks sensitive values while they are typed.
func echoMode(sensitive bool) textinput.EchoMode {
	if sensitive {
		return textinput.EchoPassword
	}
	return textinput.EchoNormal
}

func cycle(choices []string, current string, forward bool) string {
	i := slices.Index(choices, current)
	switch {
	case i < 0:
		return choices[0]
	case forward:
		return choices[(i+1)%len(choices)]
	default:
		return choices[(i+len(choices)-1)%len(choices)]
	}
}

func (m *Model) logKey(name string) {
	rows := m.logRows()
	switch name {
	case keyFollow:
		m.follow = !m.follow
		m.logTop = m.tailTop(rows)
	case keyUp, keyVimUp:
		m.scrollLog(-1, rows)
	case keyDown, keyVimDown:
		m.scrollLog(1, rows)
	case keyPageUp:
		m.scrollLog(-rows, rows)
	case keyPageDown:
		m.scrollLog(rows, rows)
	}
}

// scrollLog moves the first visible entry; reaching the end resumes follow.
func (m *Model) scrollLog(delta, rows int) {
	if m.follow {
		m.logTop = m.tailTop(rows)
	}
	tail := m.tailTop(rows)
	m.logTop = min(max(m.logTop+delta, 0), tail)
	m.follow = m.logTop == tail && delta > 0
}

func (m *Model) tailTop(rows int) int {
	return max(len(m.app.Log.View().Entries)-rows, 0)
}

// settle adjusts renderer state after the models changed: a closed drift
// detail stays closed when its target disappears, and a scrolled log keeps
// showing the same entries when the full ring buffer drops its oldest ones.
func (m *Model) settle() {
	if m.app.Drift.View().Detail == nil {
		m.detail = false
	}
	dropped := m.app.Log.View().Dropped
	if !m.follow {
		m.logTop = max(m.logTop-(dropped-m.dropped), 0)
	}
	m.dropped = dropped
}
