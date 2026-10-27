package tui

import (
	"testing"
)

// TestAgentsManifestUpgradeEnablesHosts upgrades the schema-1 manifest from
// the Agents screen, after a preview, and then offers the Machines step.
func TestAgentsManifestUpgradeEnablesHosts(t *testing.T) {
	t.Parallel()
	d := drive(t, withAgents(newFake()), 100, 40)
	d.press("5")
	d.expect("all hosts; choosing")
	d.clickText("[ Upgrade the manifest ]")
	d.expect("Upgrade kitbag.toml to schema 2", "What changes:", "set schema = 2", "! every machine that syncs this repository must run kitbag 0.1.4",
		"Checked: laptop, mac, any other machine keep the same agent instances.", "+schema = 2", "> [ Cancel ]", "[ Upgrade and publish ]")
	d.golden("agents-upgrade-preview")
	if _, _, applies := d.client.agents.calls(); len(applies) != 0 || d.client.agents.snapshot.Schema != 1 {
		t.Fatal("the preview wrote something")
	}
	// A stray Enter cancels; nothing is written.
	d.press(keyEnter)
	d.refuse("Upgrade kitbag.toml to schema 2")
	if d.client.agents.snapshot.Schema != 1 {
		t.Fatal("cancel upgraded the manifest")
	}
	d.clickText("[ Upgrade the manifest ]")
	d.press(keyRight, keyEnter)
	d.expect("kitbag.toml uses schema 2 now; you can choose machines.", "Published upgrade manifest to schema 2 as 0123456789ab.")
	d.press(keyEnter)
	d.refuse("Upgrade kitbag.toml to schema 2", "[ Upgrade the manifest ]", "needs manifest schema 2")

	d.press(keyAdd)
	d.expect("Step 1/8")
	d.press(keyEnter, keyEnter, "work", keyEnter, "~/.claude-work", keyEnter)
	d.expect("Machines", "Step 4/8", "(*) This machine only (laptop)", "( ) All machines", "( ) Choose machines…", "Machines named in kitbag.toml: laptop, mac")
	d.golden("agents-hosts-step")
}
