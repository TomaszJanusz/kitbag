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

type dialogWindow struct {
	inputs []textinput.Model
	view   model.DialogView
	focus  int
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
	w.setFocus(w.focus, 1)
}

// Replace updates a step by stable dialog ID without opening another window.
// Focus follows the same field/button index when it remains enabled.
func (s *WindowStack) Replace(view model.DialogView) {
	for i, w := range s.windows {
		if w.view.ID == view.ID {
			focus := w.focus
			tail := s.windows[i+1:]
			s.windows = s.windows[:i]
			s.Push(view)
			s.top().reuseInputs(w)
			s.top().setFocus(focus, 1)
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

func (w *dialogWindow) enabled(index int) bool {
	if index < 0 {
		return false
	}
	if index < len(w.view.Fields) {
		return w.view.Fields[index].Enabled
	}
	index -= len(w.view.Fields)
	return index < len(w.view.Buttons) && w.view.Buttons[index].Enabled
}

func (w *dialogWindow) setFocus(index, delta int) {
	count := len(w.view.Fields) + len(w.view.Buttons)
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
	switch msg.String() {
	case keyEscape:
		s.Pop()
		return w.view.Cancel
	case keyNext, keyDown:
		w.setFocus(w.focus+1, 1)
	case keyPrevious, keyUp:
		w.setFocus(w.focus-1, -1)
	case keyLeft, keyRight:
		if w.focus >= len(w.inputs) {
			delta := 1
			if msg.String() == keyLeft {
				delta = -1
			}
			w.setFocus(w.focus+delta, delta)
		} else {
			return w.edit(msg)
		}
	case keyEnter:
		return s.activate(w)
	default:
		return w.edit(msg)
	}
	return nil
}

func (s *WindowStack) activate(w *dialogWindow) model.Msg {
	index := w.focus - len(w.inputs)
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

func (w *dialogWindow) edit(msg tea.KeyMsg) model.Msg {
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
