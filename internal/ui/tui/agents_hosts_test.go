package tui

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// TestAgentsChooseMachines types a host list on the Machines step, sees the
// spelling warning and the preview naming the machines, and the published
// instance lists them in its detail.
func TestAgentsChooseMachines(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyAdd, keyEnter, "work", keyEnter, "~/.claude-work", keyEnter)
	d.expect("Machines", "Step 4/8", "(*) This machine only (laptop)")
	d.clickText("( ) Choose machines…")
	d.expect("(*) Choose machines…", "Host names separated by commas", "Enter at least one host name.", "[ Next (unavailable) ]")
	// The field sits above the choices: three steps back from the chosen one.
	d.press("shift+tab", "shift+tab", "shift+tab", "mac, macbok")
	d.expect("Machines: > mac, macbok")
	d.expect("macbok is neither this machine nor named in kitbag.toml; check its spelling.")
	d.golden("agents-hosts-pick-wide")
	d.clickText("[ Next ]")
	d.expect("Step 5/8")
	d.press(keyEnter)
	d.expect("Step 6/8", "Used only on mac, macbok; other machines ignore this instance.")
	d.press(keyEnter, keyEnter)
	d.expect("Step 8/8", "Published add claude instance work")
	d.press(keyEnter)
	d.clickText("claude/work")
	d.expect("only on mac, macbok")
	_, previews, _ := d.client.agents.calls()
	if len(previews) != 1 || previews[0].Hosts != "mac,macbok" || previews[0].AllHosts {
		t.Fatalf("previews=%+v", previews)
	}
}

// TestAgentsEditToAllMachines removes an instance's restriction.
func TestAgentsEditToAllMachines(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 120, 35)
	d.press("5", keyDown, keyEnter, keyEnter)
	d.expect("Edit claude/second: Machines", "(*) Keep the current machines (only on laptop)")
	d.clickText("( ) All machines")
	d.expect("(*) All machines")
	d.clickText("[ Next ]")
	d.press(keyEnter)
	d.expect("Used on every machine again.")
	_, previews, _ := d.client.agents.calls()
	if len(previews) != 1 || previews[0].Op != model.AgentEdit || !previews[0].AllHosts || previews[0].Hosts != "" {
		t.Fatalf("previews=%+v", previews)
	}
}
