package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// WindowStack owns terminal focus only. Dialog data, validation and wizard
// steps come from shared models. Its zero value is ready to use.
type WindowStack struct{ windows []*dialogWindow }

// dialogWindow is one open window. Focus indexes count fields, options and
// buttons in that order. moved records that the user moved the focus, so a
// reprojection of the same step keeps it; scroll is the first content line
// shown while a button has the focus.
type dialogWindow struct {
	inputs []textinput.Model
	view   model.DialogView
	focus  int
	scroll int
	moved  bool
}

// Push opens a dialog above the current one, preserving underlying focus.
func (s *WindowStack) Push(view model.DialogView) {
	w := &dialogWindow{view: view, focus: view.InitialFocus}
	for _, field := range view.Fields {
		input := textinput.New()
		input.Prompt = marker
		input.Cursor.SetMode(cursor.CursorStatic)
		input.EchoMode = echoMode(field.Sensitive)
		input.SetValue(field.Value)
		w.inputs = append(w.inputs, input)
	}
	s.windows = append(s.windows, w)
	// A disabled initial button (Next while a preview loads) falls back to
	// the one before it (Back), never forward around to Cancel.
	w.setFocus(w.focus, -1)
}

// Replace updates a step by stable dialog ID without opening another window.
// Focus follows the same field/button index when the user moved it and it
// remains enabled; a new step starts at its initial focus and the top.
func (s *WindowStack) Replace(view model.DialogView) {
	for i, w := range s.windows {
		if w.view.ID == view.ID {
			tail := append([]*dialogWindow{}, s.windows[i+1:]...)
			s.windows = s.windows[:i]
			s.Push(view)
			next := s.top()
			next.reuseInputs(w)
			if w.moved && w.view.Step == view.Step {
				next.moved, next.scroll = true, w.scroll
				next.setFocus(w.focus, 1)
			} else if w.view.Step == view.Step {
				next.scroll = w.scroll
			}
			s.windows = append(s.windows, tail...)
			return
		}
	}
	s.Push(view)
}

// Pop closes only the top dialog and restores the previous window's focus.
func (s *WindowStack) Pop() {
	if len(s.windows) > 0 {
		last := len(s.windows) - 1
		s.windows[last] = nil
		s.windows = s.windows[:last]
	}
}

// Len reports the number of open windows.
func (s *WindowStack) Len() int { return len(s.windows) }

func (s *WindowStack) top() *dialogWindow {
	if s.Len() == 0 {
		return nil
	}
	return s.windows[s.Len()-1]
}

// buttons is the focus index of the first button.
func (w *dialogWindow) buttons() int { return len(w.view.Fields) + len(w.view.Options) }

// option is the option index with the focus, or -1.
func (w *dialogWindow) option() int {
	if w.focus >= len(w.view.Fields) && w.focus < w.buttons() {
		return w.focus - len(w.view.Fields)
	}
	return -1
}

func (w *dialogWindow) enabled(index int) bool {
	switch {
	case index < 0:
		return false
	case index < len(w.view.Fields):
		return w.view.Fields[index].Enabled
	case index < w.buttons():
		return true
	}
	index -= w.buttons()
	return index < len(w.view.Buttons) && w.view.Buttons[index].Enabled
}

func (w *dialogWindow) setFocus(index, delta int) {
	count := w.buttons() + len(w.view.Buttons)
	w.focus = -1
	for range count {
		index = (index%count + count) % count
		if w.enabled(index) {
			w.focus = index
			break
		}
		index += delta
	}
	for i := range w.inputs {
		if i == w.focus {
			w.inputs[i].Focus()
		} else {
			w.inputs[i].Blur()
		}
	}
}

// Key consumes all keys while a window is open. The returned intent belongs
// to the shared model. Enter on a field activates the final button when enabled
// (Next/Finish in a form); Enter on a button activates that button.
func (s *WindowStack) Key(msg tea.KeyMsg) model.Msg {
	w := s.top()
	if w == nil {
		return nil
	}
	switch name := msg.String(); name {
	case keyEscape:
		s.Pop()
		return w.view.Cancel
	case keyNext, keyDown:
		w.moved = true
		w.setFocus(w.focus+1, 1)
	case keyPrevious, keyUp:
		w.moved = true
		w.setFocus(w.focus-1, -1)
	case keyLeft, keyRight:
		delta := 1
		if name == keyLeft {
			delta = -1
		}
		return w.step(msg, delta)
	case keyEnter:
		return s.activate(w)
	default:
		return w.edit(msg)
	}
	return nil
}

// step moves the cursor in a field, chooses the neighbouring option like a
// stepper, or moves between buttons.
func (w *dialogWindow) step(msg tea.KeyMsg, delta int) model.Msg {
	if w.focus >= 0 && w.focus < len(w.inputs) {
		return w.edit(msg)
	}
	w.moved = true
	if option := w.option(); option >= 0 {
		option = min(max(option+delta, 0), len(w.view.Options)-1)
		w.focus = len(w.view.Fields) + option
		return w.view.Options[option].Intent
	}
	if w.focus >= w.buttons() {
		w.setFocus(w.focus+delta, delta)
	}
	return nil
}

// choose dispatches an option and moves the focus to the final button
// (Next), which the reprojection keeps when the choice made it enabled.
func (w *dialogWindow) choose(option int) model.Msg {
	w.moved = true
	w.focus = w.buttons() + len(w.view.Buttons) - 1
	return w.view.Options[option].Intent
}

func (s *WindowStack) activate(w *dialogWindow) model.Msg {
	if option := w.option(); option >= 0 {
		return w.choose(option)
	}
	index := w.focus - w.buttons()
	if w.focus >= 0 && index >= 0 && w.enabled(w.focus) {
		return s.buttonIntent(w, w.view.Buttons[index])
	}
	if len(w.view.Buttons) > 0 {
		button := w.view.Buttons[len(w.view.Buttons)-1]
		if button.Enabled {
			return s.buttonIntent(w, button)
		}
	}
	return nil
}

// edit types into the focused field; Space on an option chooses it.
func (w *dialogWindow) edit(msg tea.KeyMsg) model.Msg {
	if option := w.option(); option >= 0 && msg.String() == keySpace {
		return w.view.Options[option].Intent
	}
	if w.focus < 0 || w.focus >= len(w.inputs) {
		return nil
	}
	input := &w.inputs[w.focus]
	before := input.Value()
	*input, _ = input.Update(msg)
	if before == input.Value() {
		return nil
	}
	return model.EditDialogField{Dialog: w.view.ID, Field: w.view.Fields[w.focus].ID, Value: input.Value()}
}

func (s *WindowStack) buttonIntent(w *dialogWindow, button model.Control) model.Msg {
	if w.view.CloseOnAction {
		s.Pop()
	}
	return button.Intent
}

func (w *dialogWindow) reuseInputs(previous *dialogWindow) {
	for i, field := range w.view.Fields {
		for j, old := range previous.view.Fields {
			if field.ID == old.ID && field.Value == previous.inputs[j].Value() && field.Sensitive == old.Sensitive {
				w.inputs[i] = previous.inputs[j]
			}
		}
	}
}

// Remove forgets a model-closed dialog without canceling other windows.
func (s *WindowStack) Remove(id string) {
	for i, w := range s.windows {
		if w.view.ID == id {
			copy(s.windows[i:], s.windows[i+1:])
			s.windows[len(s.windows)-1] = nil
			s.windows = s.windows[:len(s.windows)-1]
			return
		}
	}
}
