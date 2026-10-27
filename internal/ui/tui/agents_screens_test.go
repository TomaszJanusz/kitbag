package tui

import "testing"

type agentSize struct {
	name          string
	width, height int
}

// agentSizes are the golden screen sizes of the Agents tab and wizard.
func agentSizes() []agentSize {
	return []agentSize{{"wide", 120, 35}, {"medium", 100, 30}, {"narrow", 60, 20}}
}

// TestAgentsAddScreens records the tab and every add step at each size.
func TestAgentsAddScreens(t *testing.T) {
	t.Parallel()
	for _, size := range agentSizes() {
		t.Run(size.name, func(t *testing.T) {
			t.Parallel()
			d := drive(t, withAgents(newFake()), size.width, size.height)
			d.press("5")
			d.expect("kitbag › Agents", "claude/main")
			d.golden("agents-tab-" + size.name)
			d.press(keyDown, keyDown)
			d.expect("[found, not declared]", "/srv/claude")
			d.press(keyUp, keyUp)
			d.press(keyProbe, keyAdd)
			d.expect("Step 1/8", "(*) claude")
			d.golden("agents-add-kind-" + size.name)
			d.press(keyEnter, "work")
			d.expect("Step 2/8", "work")
			d.golden("agents-add-name-" + size.name)
			d.press(keyEnter)
			d.expect("Step 3/8", "Directories found on this machine:", "/home/user/.claude_3")
			d.golden("agents-add-dir-" + size.name)
			d.press(keyDown, keyDown, keyEnter)
			d.expect("/home/user/.claude_3")
			d.press(keyEnter)
			d.expect("Step 4/8", "(*) This machine only (laptop)")
			d.golden("agents-add-hosts-" + size.name)
			d.press(keyEnter)
			d.expect("Step 5/8", "(*) Default (merge shared settings)")
			d.golden("agents-add-settings-" + size.name)
			d.press(keyEnter)
			d.expect("Step 6/8", "Edit the shared manifest", "Used only on laptop")
			d.golden("agents-add-summary-" + size.name)
			d.press(keyPageDown)
			d.expect("+[targets.claude.instances.work]")
			d.golden("agents-add-preview-" + size.name)
			d.press(keyEnter)
			d.expect("Step 7/8", "> [ Apply ]", "[ Apply ]")
			d.golden("agents-add-confirm-" + size.name)
			d.press(keyEnter)
			d.expect("Step 8/8", "9b8c7d6e5f4a", "Sync succeeded, 2 changed", "The agents list was reloaded.")
			d.golden("agents-add-result-" + size.name)
			d.press(keyEnter, keyDown, keyDown)
			d.expect("claude/work")
			d.refuse("Step 8/8")
			if d.m.windows.Len() != 0 || d.step() != "" {
				t.Fatalf("result did not close: %d windows", d.m.windows.Len())
			}
			_, previews, applies := d.client.agents.calls()
			if len(previews) != 1 || len(applies) != 1 || applies[0] != "r0" || previews[0].Dir != fakeHome+"/.claude_3" {
				t.Fatalf("previews=%+v applies=%v", previews, applies)
			}
		})
	}
}
