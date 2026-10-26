package tui

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

// button is renderer-owned presentation; activation always uses Control.Intent.
// A two-cell focus gutter keeps geometry stable as keyboard focus changes.
type button struct {
	control          model.Control
	focused, primary bool
}

func (m *Model) buttonView(b button, width int) string {
	label := m.say(b.control.Label)
	prefix := indent
	if b.focused && b.control.Enabled {
		prefix = marker
	}
	style := m.styles.secondary
	if b.primary {
		style = m.styles.primary
	}
	if !b.control.Enabled {
		label = m.say(text.New(text.ControlDisabled, text.Plain(label)))
		style = m.styles.faint
	}
	if m.styles.mono || !b.primary {
		label = "[ " + label + " ]"
	} else {
		label = "  " + label + "  "
	}
	label = ansi.Truncate(label, max(width-len(prefix), 1), ellipsis)
	return prefix + style.Render(label)
}

func primaryButton(view model.DialogView, index int) bool {
	if len(view.Buttons) == 1 {
		return true
	}
	if view.Step > 0 {
		return index == len(view.Buttons)-1
	}
	return index == 0
}
