package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestAgentsListNavigationAndProbe(t *testing.T) {
	t.Parallel()
	s := start(t, withAgents(newFake()))
	s.press("5")
	s.waitFor("kitbag › Agents", "2 declared instances, 1 other directories found", "claude/main")
	s.press(keyDown)
	s.waitFor("Directory state  missing", "warns and skips it until the directory exists")
	s.press(keyVimDown)
	s.waitFor("cannot be declared", "leaves it alone; it cannot be declared")
	s.press(keyProbe)
	s.waitFor("3 other directories found", "/home/user/.claude_3", "found by name")
	screen := s.final()
	lists, previews, applies := s.client.agents.calls()
	if lists < 2 || len(previews) != 0 || len(applies) != 0 || !strings.Contains(screen, "> /srv/claude") {
		t.Fatalf("lists=%d previews=%v applies=%v\n%s", lists, previews, applies, screen)
	}
}

// TestAgentsAddWizardFlow types every draft through the real program loop.
func TestAgentsAddWizardFlow(t *testing.T) {
	t.Parallel()
	s := start(t, withAgents(newFake()))
	s.press("5")
	s.waitFor("claude/main")
	s.press(keyAdd)
	s.waitFor("Step 1/8", "> [ Next ]")
	s.press(keyEnter)
	s.waitFor("Step 2/8", "Instance name: >")
	s.press("new")
	s.waitFor("Instance name: > new")
	s.press(keyEnter)
	s.waitFor("Step 3/8")
	s.press("~/.claude-new")
	s.waitFor("Configuration directory: > ~/.claude-new")
	s.press(keyEnter)
	s.waitFor("Step 4/8", "(*) This machine only (laptop)", "( ) All machines")
	s.press(keyEnter)
	s.waitFor("Step 5/8")
	s.press("shift+tab", "shift+tab", "shift+tab", keyEnter)
	s.waitFor("(*) Do not merge shared settings")
	s.press(keyEnter)
	s.waitFor("Step 6/8", "Local configuration directory")
	s.press(keyPageDown)
	s.waitFor("+dir = \"~/.claude-new\"")
	s.press(keyEnter)
	s.waitFor("Step 7/8", "> [ Apply ]")
	s.press(keyEnter)
	s.waitFor("Step 8/8", "Sync succeeded, 2 changed")
	s.press(keyEnter)
	s.waitFor("claude/new  /home/user/.claude-new")
	if screen := s.final(); !strings.Contains(screen, "3 declared instances") || s.m.windows.Len() != 0 {
		t.Fatalf("list not refreshed:\n%s", screen)
	}
	_, previews, applies := s.client.agents.calls()
	off := false
	want := model.AgentChange{Op: model.AgentAdd, Kind: "claude", Name: "new", Dir: "~/.claude-new", Settings: &off, Hosts: "laptop"}
	if len(previews) != 1 || len(applies) != 1 || !sameChange(previews[0], want) {
		t.Fatalf("previews=%+v applies=%v", previews, applies)
	}
}

// sameChange compares changes by value, including the settings choice.
func sameChange(got, want model.AgentChange) bool {
	settings := (got.Settings == nil) == (want.Settings == nil) && (got.Settings == nil || *got.Settings == *want.Settings)
	got.Settings, want.Settings = nil, nil
	return settings && got == want
}

func TestAgentsCancelAsksBeforeDiscarding(t *testing.T) {
	t.Parallel()
	s := start(t, withAgents(newFake()))
	s.press("5")
	s.waitFor("claude/main")
	s.press(keyAdd, keyEscape, keyAdd, keyEnter, "draft", keyEscape)
	s.waitFor("Discard this change?", "Nothing has been written.", "> [ No ]")
	s.press(keyEnter)
	s.waitFor("Instance name:", "draft", "> [ Next ]")
	s.press(keyEscape)
	s.waitFor("Discard this change?")
	s.press(keyLeft, keyEnter)
	screen := s.final()
	_, previews, applies := s.client.agents.calls()
	if s.m.windows.Len() != 0 || s.m.app.AgentForm.Open() || len(previews) != 0 || len(applies) != 0 || strings.Contains(screen, "Step 2/8") {
		t.Fatalf("windows=%d previews=%v applies=%v\n%s", s.m.windows.Len(), previews, applies, screen)
	}
}

// TestAgentsBusyWhileApplying holds the apply: the window says so, refuses
// to close and shows the result once the daemon answers.
func TestAgentsBusyWhileApplying(t *testing.T) {
	t.Parallel()
	client := withAgents(newFake())
	gate := make(chan struct{})
	client.agents.gate = gate
	opened := true
	release := func() {
		if opened {
			opened = false
			close(gate)
		}
	}
	s := start(t, client)
	t.Cleanup(release)
	s.press("5")
	s.waitFor("claude/second")
	s.press(keyDown, keyRemove)
	s.waitFor("Step 1/3", "Edit the shared manifest")
	s.press(keyPageDown)
	s.waitFor("-[targets.claude.instances.second]")
	s.press(keyEnter)
	s.waitFor("Step 2/3", "> [ Apply ]")
	s.press(keyEnter)
	s.waitFor("Applying and publishing…")
	// Esc and an outside click are refused while applying: the window stays
	// and shows the result below.
	s.press(keyEscape)
	s.click(0, 0)
	release()
	s.waitFor("Step 3/3", "Handed back ~/.claude_2")
	s.press(keyEnter)
	screen := s.final()
	if strings.Contains(screen, "claude/second") || s.m.windows.Len() != 0 {
		t.Fatalf("removed instance still listed:\n%s", screen)
	}
}

func TestAgentsResizeWithWizardOpen(t *testing.T) {
	t.Parallel()
	s := start(t, withAgents(newFake()))
	s.press("5")
	s.waitFor("claude/main")
	s.press(keyAdd, keyEnter, "kept")
	s.waitFor("Instance name: > kept")
	s.tm.Send(tea.WindowSizeMsg{Width: 60, Height: 20})
	s.tm.Send(tea.WindowSizeMsg{Width: 30, Height: 8})
	s.waitFor("Enlarge the terminal")
	s.tm.Send(tea.WindowSizeMsg{Width: 120, Height: 35})
	s.press("!")
	s.waitFor("Instance name: > kept!", "Use up to 64 lowercase letters")
	screen := s.final()
	assertScreenFits(t, screen, 120, 35)
	if !strings.Contains(screen, "Step 2/8") {
		t.Fatal(screen)
	}
}

func TestAgentsValidationErrors(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 100, 30)
	d.press("5", keyAdd, keyDown, keyDown, keyDown, keyDown, keyDown, keySpace)
	d.expect("> (*) generic", "Generic agents need explicit roots", "[ Next (unavailable) ]")
	d.press(keyUp, keyUp, keyUp, keyUp, keySpace)
	d.expect("> (*) claude")
	d.refuse("Generic agents need explicit roots")
	d.press(keyEnter, keyEnter, "main")
	d.expect("Target claude already has an instance named main.", "[ Next (unavailable) ]")
	d.press(keyEnter)
	d.expect("Step 2/8")
	d.press("backspace", "backspace", "backspace", "backspace", "Work")
	d.expect("Use up to 64 lowercase letters")
	d.golden("agents-invalid-name-medium")
	d.press("backspace", "backspace", "backspace", "backspace", "work", keyEnter, "/srv/claude")
	d.expect("/srv/claude is outside the home directory; it cannot be declared.")
	d.press(keyEnter)
	d.expect("Step 3/8")
	for range len("/srv/claude") {
		d.press("backspace")
	}
	d.press("~/.CLAUDE_2")
	d.expect("~/.CLAUDE_2 is already declared as claude/second.")
	d.golden("agents-invalid-dir-medium")
	d.press(keyEnter)
	if _, previews, applies := d.client.agents.calls(); d.step() != model.AgentStepDir || len(previews) != 0 || len(applies) != 0 {
		t.Fatalf("invalid draft advanced: step=%s previews=%v applies=%v", d.step(), previews, applies)
	}
}

func TestAgentsEditDirectory(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyDown, keyEnter)
	d.expect("Edit claude/second: Configuration directory", "Step 1/6", "Now /home/user/.claude_2; leave empty to keep it.")
	d.golden("agents-edit-dir-wide")
	d.press(keyNext, "~/.claude_9", keyEnter)
	d.expect("Step 2/6", "(*) Keep the current machines")
	d.golden("agents-edit-hosts-wide")
	d.press(keyEnter)
	d.expect("Step 3/6", "(*) Keep the current setting")
	d.press(keyEnter)
	d.expect("Step 4/6", `-dir = "~/.claude_2"`, `+dir = "~/.claude_9"`, "~/.claude_2 is handed back")
	d.golden("agents-edit-preview-wide")
	d.press(keyEnter, keyEnter)
	d.expect("Step 6/6", "Handed back ~/.claude_2", "discard ~/.claude_2/CLAUDE.md")
	d.press(keyEnter)
	d.expect("claude/second  /home/user/.claude_9")
	if _, previews, _ := d.client.agents.calls(); previews[0].Op != model.AgentEdit || previews[0].Target != "claude" || previews[0].Name != "second" || previews[0].Settings != nil || previews[0].Hosts != "" || previews[0].AllHosts {
		t.Fatalf("edit change %+v", previews[0])
	}
}

func TestAgentsRemoveNeedsConfirmation(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyDown, keyRemove)
	d.expect("Remove claude/second: What will change", "Step 1/3", "-[targets.claude.instances.second]")
	d.golden("agents-remove-preview-wide")
	d.press(keyEnter)
	d.expect("Step 2/3", "> [ Apply ]")
	d.golden("agents-remove-confirm-wide")
	// Back remains available explicitly.
	d.press(keyLeft, keyEnter)
	d.expect("Step 1/3")
	if _, _, applies := d.client.agents.calls(); len(applies) != 0 {
		t.Fatalf("applied without confirmation: %v", applies)
	}
	d.press(keyEnter, keyEnter)
	d.expect("Step 3/3", "Published remove claude instance second as 9b8c7d6e5f4a.")
	d.press(keyEnter)
	d.refuse("claude/second")
	if instances := d.client.agents.instances(); len(instances) != 2 {
		t.Fatalf("instances=%+v", instances)
	}
}

// addToConfirm drives an add of claude/extra at ~/.claude_3 to the confirm
// step with the Apply button focused.
func addToConfirm(d *driver) {
	d.t.Helper()
	d.press("5", keyAdd, keyEnter, "extra", keyEnter, "~/.claude_3", keyEnter, keyEnter, keyEnter)
	d.expect("Step 6/8")
	d.press(keyEnter)
	d.expect("Step 7/8", "> [ Apply ]")
}

func TestAgentsPullRequestModeWritesNothing(t *testing.T) {
	t.Parallel()
	client := withAgents(newFake())
	client.agents.mode = "pr"
	d := drive(t, client, 120, 35)
	addToConfirm(d)
	d.expect("Not written: publish mode is pull request; the commands to propose it are shown.")
	d.press(keyEnter)
	d.expect("Not written: publish mode is pr for this host", "To propose it:", "switch -c kitbag/claude-extra", "push -u origin kitbag/claude-extra")
	d.golden("agents-result-pr-wide")
	d.press(keyEnter)
	d.refuse("claude/extra")
	if instances := d.client.agents.instances(); len(instances) != 3 {
		t.Fatalf("pr mode changed the listing: %+v", instances)
	}
}

func TestAgentsNotPublishedResult(t *testing.T) {
	t.Parallel()
	client := withAgents(newFake())
	client.agents.state = model.PublishNotPublished
	d := drive(t, client, 120, 35)
	addToConfirm(d)
	d.press(keyEnter)
	d.expect("! Committed 4f2a9c1d0b7e on this machine but not published: push rejected", "pull --rebase", "To propose it:")
	d.refuse("✓ ")
	d.golden("agents-result-not-published-wide")
}

func TestAgentsFailedApplyStaysOnConfirm(t *testing.T) {
	t.Parallel()
	client := withAgents(newFake())
	client.agents.applyErr = &model.Error{Kind: model.ErrBusy, Detail: "another kitbag operation holds the state lock"}
	d := drive(t, client, 100, 30)
	addToConfirm(d)
	d.press(keyEnter)
	d.expect("Step 7/8", "✗ Another kitbag operation is running; try again shortly: another kitbag")
	d.client.agents.set(func(a *agentFake) { a.applyErr = nil })
	d.press(keyEnter)
	d.expect("Step 8/8", "Published add claude instance extra")
}

func TestAgentsRefusedChangeShowsDetail(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyAdd, keyDown, keyDown, keyEnter, keyEnter, "work", keyEnter, "~/.codex-work", keyEnter, keyEnter, "shift+tab", "shift+tab", "shift+tab", keyLeft, keyEnter)
	d.expect("(*) Merge shared settings")
	d.press(keyEnter)
	d.expect("Step 6/8", "✗ The configuration or request is invalid: target codex declares no settings fragment", "[ Next (unavailable) ]")
	d.golden("agents-refused-wide")
	// With Next unavailable the focus rests on Back: Enter returns to fix it.
	d.press(keyEnter)
	d.expect("Step 5/8")
	d.refuse("declares no settings fragment")
	if _, _, applies := d.client.agents.calls(); len(applies) != 0 {
		t.Fatalf("refused change applied: %v", applies)
	}
}

// TestAgentsStalePreviewIsNotApplied changes kitbag.toml between preview and
// apply: the apply is refused, a new preview is shown and only its revision
// is applied.
func TestAgentsStalePreviewIsNotApplied(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	addToConfirm(d)
	d.client.agents.bump()
	d.press(keyEnter)
	d.expect("Step 6/8", "kitbag.toml changed since the preview. Review the new preview.")
	d.golden("agents-stale-wide")
	d.press(keyEnter, keyEnter)
	d.expect("Step 8/8")
	if _, previews, applies := d.client.agents.calls(); len(previews) != 2 || strings.Join(applies, ",") != "r0,r1" {
		t.Fatalf("previews=%d applies=%v", len(previews), applies)
	}
}

func TestAgentsDiscardScreen(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyAdd, keyEnter, "draft", keyEscape)
	d.expect("Discard this change?", "> [ No ]")
	d.golden("agents-discard-wide")
	d.press(keyEscape)
	d.expect("Instance name:", "draft", "> [ Next ]")
	d.refuse("Discard this change?")
}

func TestAgentsMouse(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.clickText("Agents")
	d.clickText("claude/second  /home")
	d.expect("Directory state  missing")
	d.refuse("Step 1/6")
	d.clickText("claude/second  /home")
	d.expect("Step 1/6", "Edit claude/second")
	d.clickText("[ Cancel ]")
	d.refuse("Step 1/6")
	d.send(tea.MouseMsg{X: 3, Y: 6, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	d.expect("Directory state  outside the home directory")
	d.refuse("[ Remove (unavailable) ]", "[ Edit (unavailable) ]")
	d.press(keyRemove, keyEdit)
	d.refuse("Step 1/3")
	d.clickText("[ Look for more directories ]")
	d.expect("/home/user/.claude_3")
	d.clickText("[ Add this directory ]")
	d.expect("Step 1/8")
	d.clickText("( ) codex")
	d.expect("(*) codex")
	d.clickText("[ Next ]")
	d.expect("Step 2/8")
	d.click(0, 0)
	d.expect("Discard this change?")
	d.clickText("[ Yes ]")
	d.refuse("Step 2/8", "Discard this change?")
	d.clickText("claude/main  /home")
	d.clickText("[ Remove ]")
	d.expect("Step 1/3", "Remove claude/main")
	d.clickText("[ Next ]")
	d.clickText("[ Back ]")
	d.expect("Step 1/3")
	if _, _, applies := d.client.agents.calls(); len(applies) != 0 {
		t.Fatalf("a click applied a change: %v", applies)
	}
}

func TestAgentsWindowScrollsLongPreview(t *testing.T) {
	t.Parallel()
	client := withAgents(newFake())
	d := drive(t, client, 60, 15)
	d.press("5", keyDown, keyEnter, keyNext, "~/.claude_9", keyEnter, keyEnter, keyEnter)
	d.expect("Step 4/6", "scroll ↑/↓")
	d.refuse(`+dir = "~/.claude_9"`)
	d.press(keyPageDown, keyPageDown, keyPageDown, keyPageDown)
	d.expect(`+dir = "~/.claude_9"`)
	d.send(tea.MouseMsg{X: 30, Y: 10, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	d.press(keyPageUp)
	d.refuse(`+dir = "~/.claude_9"`)
}

func TestAgentWizardReadyStepsFocusPrimaryButton(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.m.dispatch(model.OpenAgentForm{Op: model.AgentAdd, Kind: "claude", Name: "ready", Dir: "~/.claude-ready"})
	d.settle()
	for _, step := range []model.AgentStep{model.AgentStepKind, model.AgentStepName, model.AgentStepDir, model.AgentStepHosts, model.AgentStepSettings, model.AgentStepPreview, model.AgentStepConfirm, model.AgentStepResult} {
		w := d.m.windows.top()
		if d.step() != step || w == nil {
			t.Fatalf("step %s, want %s", d.step(), step)
		}
		index := w.focus - w.buttons()
		if index != len(w.view.Buttons)-1 || index < 0 || !w.view.Buttons[index].Enabled {
			t.Fatalf("step %s focused %d, want enabled primary button", step, w.focus)
		}
		if _, ok := w.view.Buttons[index].Intent.(model.AgentFormNext); !ok {
			t.Fatalf("step %s focused intent %#v", step, w.view.Buttons[index].Intent)
		}
		d.press(keyEnter)
	}
	if d.m.windows.Len() != 0 {
		t.Fatal("Close did not close the result")
	}
}

func TestAgentWizardTypingKeepsFocusAndCursorWhenNextBecomesValid(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyAdd, keyEnter)
	// Type one character at a time: the first enables Next, later edits must
	// stay in the field, including edits after explicit cursor movement.
	d.press("w", "o", "k", keyLeft, "r")
	d.expect("Instance name: > work")
	if w := d.m.windows.top(); w.focus != 0 || w.inputs[0].Position() != 3 {
		t.Fatalf("typing lost field/cursor: focus %d position %d", w.focus, w.inputs[0].Position())
	}
	d.press(keyEnter)
	d.expect("Step 3/8")
	d.press("~", "/", ".", "w", "o", "k", keyLeft, "r")
	d.expect("Configuration directory: > ~/.work")
	// Enter a valid prefilled step again, explicitly choose its field, edit,
	// and verify subsequent projections preserve that choice too.
	d.clickText("[ Back ]")
	d.expect("Step 2/8", "> [ Next ]")
	d.press(keyNext, "s")
	d.expect("Instance name: > works")
}

func TestAgentsAdoptFoundDirectory(t *testing.T) {
	t.Parallel()
	for _, trigger := range []string{"enter", "double-click", "button"} {
		t.Run(trigger, func(t *testing.T) {
			t.Parallel()
			d := drive(t, withAgents(newFake()), 120, 35)
			d.press("5", keyProbe)
			d.clickText("/home/user/.claude_3")
			d.refuse("[ Edit", "[ Remove")
			switch trigger {
			case "enter":
				d.press(keyEnter)
			case "double-click":
				d.clickText("> /home/user/.claude_3")
			case "button":
				d.clickText("[ Add this directory ]")
			}
			d.expect("Adopt claude at /home/user/.claude_3", "Step 1/6", "Instance name")
			d.walkAdoptToPreview()
			d.expect("Step 4/6", "+dir = \"~/.claude_3\"")
			d.golden("agents-adopt-preview-" + trigger)
			_, previews, applies := d.client.agents.calls()
			if len(previews) != 1 || previews[0].Kind != "claude" || previews[0].Dir != "/home/user/.claude_3" || len(applies) != 0 {
				t.Fatalf("candidate lost: previews=%+v applies=%v", previews, applies)
			}
			d.advanceToPrimary(model.AgentStepConfirm)
		})
	}
}

// expectPrimaryFocus fails unless the focused control is the last, enabled
// button of the top window, which is the wizard's primary action.
func (d *driver) expectPrimaryFocus(step model.AgentStep) {
	d.t.Helper()
	w := d.m.windows.top()
	if w == nil {
		d.t.Fatalf("step %s: no window", step)
	}
	index := w.focus - w.buttons()
	if index != len(w.view.Buttons)-1 || index < 0 || !w.view.Buttons[index].Enabled {
		d.t.Fatalf("step %s focused %d, want enabled primary button", step, w.focus)
	}
}

// walkAdoptToPreview types the instance name and walks the adopt wizard to its
// preview, checking the primary button holds focus on every step after the
// field the user typed in.
func (d *driver) walkAdoptToPreview() {
	d.t.Helper()
	d.press("work")
	d.advanceToPrimary(model.AgentStepHosts)
	d.advanceToPrimary(model.AgentStepSettings)
	d.advanceToPrimary(model.AgentStepPreview)
}

// advanceToPrimary presses Enter and expects the wizard on step with its
// primary button focused.
func (d *driver) advanceToPrimary(step model.AgentStep) {
	d.t.Helper()
	d.press(keyEnter)
	if d.step() != step {
		d.t.Fatalf("step %s, want %s", d.step(), step)
	}
	d.expectPrimaryFocus(step)
}
