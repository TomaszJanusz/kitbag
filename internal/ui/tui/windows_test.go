package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
)

func confirmFixture(id string) model.DialogView {
	return model.ConfirmDialog(id, text.Plain(id), text.Plain("Confirm this action?"), model.SyncNow{}, model.DismissDialog{ID: id})
}

func TestDialogStackFocusTrapAndEscape(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(confirmFixture("Outer"))
	s.waitFor("Outer", "> [ No ]")
	s.press(keyLeft)
	s.tm.Send(confirmFixture("Inner"))
	s.waitFor("Inner")
	s.press(keyNext, keyPrevious, "3", keyRefresh, keySync, keyQuit, keyHelp)
	s.press(keyEscape)
	s.waitFor("Outer", "> [ Yes ]")
	screen := s.final()
	if s.m.tab != statusTab || s.m.windows.Len() != 1 || strings.Contains(screen, "Inner") || !strings.Contains(screen, "> [ Yes ]") {
		t.Fatal(screen)
	}
}

func TestDialogOutsideClickCancelsOnlyTop(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(confirmFixture("Outer"))
	s.tm.Send(confirmFixture("Inner"))
	s.waitFor("Inner")
	s.click(0, 0)
	s.waitFor("Outer")
	screen := s.final()
	if s.m.windows.Len() != 1 || strings.Contains(screen, "Inner") || !strings.Contains(screen, "Outer") {
		t.Fatal(screen)
	}
}

func TestUnsetConfirmNoEscapeOutsideAndYes(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press("3", keyUnset, keyEnter)
	s.waitFor("[3 Settings]")
	s.press(keyUnset, keyEscape, keyUnset)
	s.waitFor("Remove stored setting?")
	s.click(0, 0)
	s.press(keyUnset, keyLeft, keyEnter)
	if got := s.nextWrite(); got != [2]string{"host", ""} {
		t.Fatal(got)
	}
	screen := s.final()
	if s.m.windows.Len() != 0 || strings.Contains(screen, "Remove stored setting?") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestDialogResizeScreens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"dialog-wide", 120, 35}, {"dialog-medium", 100, 30}, {"dialog-narrow", 60, 20}, {"dialog-minimum", 40, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := start(t, newFake())
			s.waitFor("2 targets changed locally")
			s.tm.Send(formFixture())
			s.waitFor("Step 2/5")
			s.press(" draft")
			s.tm.Send(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			s.press(keyNext)
			s.waitFor("> [ Back ]")
			screen := s.final()
			assertScreenFits(t, screen, tc.width, tc.height)
			if !strings.Contains(screen, "Step 2/5") || !strings.Contains(screen, "draft") {
				t.Fatal(screen)
			}
			golden(t, tc.name, screen)
		})
	}
}

func formFixture() model.DialogView {
	return model.FormDialog("wizard", text.Plain("Add agent"), 2, 5, []model.DialogField{
		{ID: "directory", Label: text.Plain("Directory"), Value: "~/.agent", Enabled: true},
	}, model.Control{Intent: model.Refresh{}, Enabled: true}, model.Control{Intent: model.SyncNow{}, Enabled: true}, model.DismissDialog{ID: "wizard"})
}

func TestFormFocusEditingAndDisabledNext(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	view := formFixture()
	view.Buttons[1].Enabled = false
	windows.Push(view)
	msg := windows.Key(keyMsg("x"))
	if got, ok := msg.(model.EditDialogField); !ok || got.Field != "directory" || got.Value != "~/.agentx" {
		t.Fatalf("%#v", msg)
	}
	if got := windows.Key(keyMsg(keyEnter)); got != nil {
		t.Fatalf("disabled Next activated: %#v", got)
	}
	windows.Key(keyMsg(keyPrevious))
	if windows.top().focus != 1 {
		t.Fatal("focus did not skip disabled Next")
	}
	windows.Key(keyMsg(keyNext))
	if windows.top().focus != 0 {
		t.Fatal("focus escaped the form")
	}
}

func TestFormReplacementKeepsDraftAndStep(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	view := formFixture()
	windows.Push(view)
	view.Fields[0].Value = "~/.agentx"
	view.Step, view.Steps = 5, 5
	view.Buttons[1].Enabled = true
	windows.Replace(view)
	if windows.Len() != 1 || windows.top().inputs[0].Value() != "~/.agentx" {
		t.Fatal("replacement lost the draft")
	}
	if got := windows.Key(keyMsg(keyEnter)); got != (model.SyncNow{}) || windows.Len() != 1 {
		t.Fatalf("Next must return an intent without closing: %#v", got)
	}
}

func TestDialogMouseFieldsButtonsAndStrays(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 120, 35)
	m.ShowDialog(formFixture())
	layout := m.windowLayout(m.windows.top())
	field, button := layout.hits[0], layout.hits[2]
	m.windows.Key(keyMsg(keyNext))
	m.mouse(tea.MouseMsg{X: field.rect.X, Y: field.rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.windows.top().focus != 0 {
		t.Fatal("field click did not focus it")
	}
	for _, action := range []tea.MouseAction{tea.MouseActionMotion, tea.MouseActionRelease} {
		m.mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: action})
	}
	if m.windows.Len() != 1 {
		t.Fatal("a stray mouse event canceled the dialog")
	}
	got := m.windowMouse(tea.MouseMsg{X: button.rect.X, Y: button.rect.Y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got != (model.SyncNow{}) {
		t.Fatalf("button intent=%#v", got)
	}
}

func TestReplacingFormPreservesCursorAndUnderlyingWindow(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	view := formFixture()
	windows.Push(view)
	windows.Key(keyMsg(keyLeft))
	windows.Push(confirmFixture("confirm"))
	windows.Replace(view)
	if windows.Len() != 2 || windows.top().view.ID != "confirm" {
		t.Fatal("replacement reordered the stack")
	}
	windows.Pop()
	edit, ok := windows.Key(keyMsg("x")).(model.EditDialogField)
	if !ok {
		t.Fatal("missing field edit")
	}
	if edit.Value != "~/.agenxt" {
		t.Fatalf("cursor reset during projection: %q", edit.Value)
	}
	windows.Remove("wizard")
	windows.Remove("missing")
	if windows.Len() != 0 {
		t.Fatal("closed model window still rendered")
	}
}

func TestNoColourDialogMasksSecretsAndMarksFocus(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 100, 30)
	view := formFixture()
	view.Fields[0].Sensitive = true
	view.Fields[0].Value = "secret-value"
	m.ShowDialog(view)
	screen := m.View()
	if ansiPattern.MatchString(screen) || strings.Contains(screen, "secret-value") || !strings.Contains(screen, "> ************") {
		t.Fatal(screen)
	}
}

func TestFormWithNoEnabledControlsTrapsInput(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	view := formFixture()
	view.Fields[0].Enabled = false
	view.Buttons[0].Enabled, view.Buttons[1].Enabled = false, false
	windows.Push(view)
	for _, key := range []string{keyNext, keyPrevious, keyUp, keyDown, keyLeft, keyRight, keyEnter, "x"} {
		if msg := windows.Key(keyMsg(key)); msg != nil {
			t.Fatalf("disabled control returned %#v", msg)
		}
	}
	if windows.top().focus != -1 {
		t.Fatal("focus reached disabled controls")
	}
	windows.Key(keyMsg(keyEscape))
	if windows.Len() != 0 {
		t.Fatal("Esc failed with disabled controls")
	}
}

func TestDialogButtonMouseConfirmsStoredUnset(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.press("3", keyUnset)
	s.waitFor("Remove stored setting?", "> [ No ]")
	// 80x24: the Yes button is on row 12 in the centered six-line dialog.
	s.click(8, 12)
	if got := s.nextWrite(); got != [2]string{"host", ""} {
		t.Fatal(got)
	}
	if screen := s.final(); strings.Contains(screen, "Remove stored setting?") {
		t.Fatal(screen)
	}
	s.noWrite()
}

func TestResizeBelowMinimumPreservesOpenDialog(t *testing.T) {
	t.Parallel()
	s := start(t, newFake())
	s.waitFor("2 targets changed locally")
	s.tm.Send(formFixture())
	s.waitFor("Step 2/5")
	s.press(" draft")
	s.tm.Send(tea.WindowSizeMsg{Width: 30, Height: 8})
	s.waitFor("Enlarge the terminal")
	s.tm.Send(tea.WindowSizeMsg{Width: 60, Height: 20})
	s.press(keyNext)
	s.waitFor("> [ Back ]", "draft")
	screen := s.final()
	if !strings.Contains(screen, "Step 2/5") || s.m.windows.Len() != 1 {
		t.Fatal(screen)
	}
}

func TestDialogKeysDoNotReachBackground(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 120, 35)
	m.ShowDialog(confirmFixture("trap"))
	for _, key := range []string{"4", keyRefresh, keySync, keyHelp, keyAdvanced} {
		m.key(keyMsg(key))
	}
	if m.tab != settingsTab || m.help || m.app.Settings.View().Expanded || m.windows.Len() != 1 {
		t.Fatal("focus escaped the dialog")
	}
}

func TestFormFocusOrderSkipsDisabledFields(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	view := formFixture()
	view.Fields = append(view.Fields, model.DialogField{ID: "disabled"}, model.DialogField{ID: "name", Enabled: true})
	windows.Push(view)
	for _, step := range []struct {
		key   string
		focus int
	}{
		{keyNext, 2},
		{keyNext, 3},
		{keyRight, 4},
		{keyNext, 0},
		{keyPrevious, 4},
		{keyUp, 3},
		{keyLeft, 2},
		{keyDown, 3},
	} {
		windows.Key(keyMsg(step.key))
		if windows.top().focus != step.focus {
			t.Fatalf("%s: focus=%d want=%d", step.key, windows.top().focus, step.focus)
		}
	}
}

func TestLargeFormStaysInsideMinimumWindow(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, MinWidth, MinHeight)
	view := formFixture()
	for range 20 {
		view.Fields = append(view.Fields, model.DialogField{ID: "extra", Label: text.Plain("Field"), Hint: text.Plain("Long field help goes here"), Enabled: true})
		view.Buttons = append(view.Buttons, model.Control{Label: text.Plain("Extra button"), Enabled: true})
	}
	m.ShowDialog(view)
	for range len(view.Fields) + len(view.Buttons) {
		m.windows.Key(keyMsg(keyNext))
		screen := m.View()
		assertScreenFits(t, screen, MinWidth, MinHeight)
		if !strings.Contains(screen, "Step 2/5") {
			t.Fatal("step indicator scrolled away")
		}
	}
}

func TestDialogTextCursorStaysVisibleAfterResize(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 120, 35)
	view := formFixture()
	view.Fields[0].Value = "START" + strings.Repeat("x", 100) + "TAIL"
	m.ShowDialog(view)
	for _, width := range []int{120, 40, 60, 100} {
		m.width = width
		screen := m.View()
		if !strings.Contains(screen, "TAIL") {
			t.Fatalf("width=%d cursor end is hidden:\n%s", width, screen)
		}
	}
	m.windows.Key(keyMsg("ctrl+a"))
	if screen := m.View(); !strings.Contains(screen, "START") {
		t.Fatal("cursor start is hidden")
	}
}

func TestDialogMouseTakesPriorityOverHelp(t *testing.T) {
	t.Parallel()
	m := loadedSettingsModel(t, 120, 35)
	m.help = true
	m.ShowDialog(confirmFixture("over-help"))
	m.mouse(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.windows.Len() != 0 || !m.help {
		t.Fatal("help swallowed the top window's outside click")
	}
}

func choiceFixture(step int, chosen int, nextEnabled bool) model.DialogView {
	view := model.FormDialog("choices", text.Plain("Choose"), step, 3, nil,
		model.Control{Intent: model.Refresh{}, Enabled: true}, model.Control{Intent: model.SyncNow{}, Enabled: nextEnabled}, model.DismissDialog{ID: "choices"})
	for i, label := range []string{"one", "two", "three"} {
		view.Options = append(view.Options, model.DialogOption{Intent: model.ChooseAgentOption{Index: i}, Label: text.Plain(label), Selected: i == chosen})
	}
	view.InitialFocus = max(chosen, 0)
	return view
}

func TestWindowOptionsChooseAndStep(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	windows.Push(choiceFixture(1, -1, false))
	if got := windows.Key(keyMsg(keySpace)); got != (model.ChooseAgentOption{Index: 0}) || windows.top().focus != 0 {
		t.Fatalf("space chose %#v, focus %d", got, windows.top().focus)
	}
	if got := windows.Key(keyMsg(keyRight)); got != (model.ChooseAgentOption{Index: 1}) || windows.top().option() != 1 {
		t.Fatalf("right stepped to %#v", got)
	}
	windows.Key(keyMsg(keyRight))
	if got := windows.Key(keyMsg(keyRight)); got != (model.ChooseAgentOption{Index: 2}) {
		t.Fatalf("stepping past the last option: %#v", got)
	}
	if got := windows.Key(keyMsg("x")); got != nil {
		t.Fatalf("a letter on an option: %#v", got)
	}
}

func TestWindowOptionEnterFocusesNext(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	windows.Push(choiceFixture(1, 2, false))
	if got := windows.Key(keyMsg(keyEnter)); got != (model.ChooseAgentOption{Index: 2}) {
		t.Fatalf("enter chose %#v", got)
	}
	// The model enables Next for the choice; the focus waits there.
	windows.Replace(choiceFixture(1, 2, true))
	if top := windows.top(); top.focus != 4 {
		t.Fatalf("focus %d after choosing, want Next", top.focus)
	}
	if got := windows.Key(keyMsg(keyEnter)); got != (model.SyncNow{}) {
		t.Fatalf("second enter: %#v", got)
	}
}

func TestWindowReplaceFocusRules(t *testing.T) {
	t.Parallel()
	var windows WindowStack
	windows.Push(choiceFixture(1, 0, true))
	windows.Key(keyMsg(keyDown))
	// A new step starts at its initial focus.
	windows.Replace(choiceFixture(2, 1, true))
	if top := windows.top(); top.focus != 1 || top.moved {
		t.Fatalf("new step focus %d moved %v", top.focus, top.moved)
	}
	// Without user movement a reprojection follows the initial focus; a
	// disabled initial button falls back to the one before it.
	view := choiceFixture(2, -1, false)
	view.InitialFocus = 4
	windows.Replace(view)
	if top := windows.top(); top.focus != 3 {
		t.Fatalf("disabled initial Next fell to %d, want Back", top.focus)
	}
	view.Buttons[1].Enabled = true
	windows.Replace(view)
	if top := windows.top(); top.focus != 4 {
		t.Fatalf("enabled Next not focused: %d", top.focus)
	}
}
