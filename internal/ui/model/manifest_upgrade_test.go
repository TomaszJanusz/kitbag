package model

import (
	"strings"
	"testing"
)

func upgradeFixture() ManifestUpgradePreview {
	return ManifestUpgradePreview{
		Revision: "r1", Title: "chore(kitbag): upgrade manifest to schema 2", MinToolVersion: "0.1.4", SchemaBefore: 1,
		Conversions:  []string{"set schema = 2"},
		Diff:         []string{"-schema = 1", "+schema = 2"},
		Warnings:     []string{"every machine must run kitbag 0.1.4 or later"},
		CheckedHosts: []string{"mac", "kitbag-undeclared-host"},
		Publication:  AgentPublication{State: PublishDryRun, Mode: "direct", Branch: "main"},
	}
}

func upgradeButtons(t *testing.T, m *ManifestUpgrade) (Control, Control) {
	t.Helper()
	d := m.Dialog()
	expect(t, d != nil && d.ID == ManifestUpgradeDialog && len(d.Buttons) == 2, "dialog %+v", d)
	checkText(t, dialogTexts(*d)...)
	return d.Buttons[0], d.Buttons[1]
}

// TestManifestUpgradePreviewThenApply shows what changes before anything is
// written and applies exactly the previewed revision.
func TestManifestUpgradePreviewThenApply(t *testing.T) {
	t.Parallel()
	m := NewManifestUpgrade()
	expect(t, m.Dialog() == nil, "closed window shown")
	preview := as[PreviewManifestUpgrade](t, only(t, m.Update(OpenManifestUpgrade{})))
	_, apply := upgradeButtons(t, m)
	expect(t, !apply.Enabled && strings.Contains(bodyText(m.Dialog()), "Preparing the upgrade preview"), "loading %q", bodyText(m.Dialog()))
	expect(t, len(m.Update(ConfirmManifestUpgrade{})) == 0, "applied without a preview")
	m.Update(ManifestUpgradePreviewed{Seq: preview.Seq + 1, Preview: upgradeFixture()})
	expect(t, m.preview == nil, "stale preview accepted")
	m.Update(ManifestUpgradePreviewed{Seq: preview.Seq, Preview: upgradeFixture()})
	body := bodyText(m.Dialog())
	for _, want := range []string{"What changes:", "set schema = 2", "every machine must run kitbag 0.1.4 or later", "Checked: mac, any other machine keep the same agent instances.", "Commit: chore(kitbag): upgrade manifest to schema 2", "Published directly to main.", "+schema = 2"} {
		expect(t, strings.Contains(body, want), "body lacks %q:\n%s", want, body)
	}
	cancel, apply := upgradeButtons(t, m)
	expect(t, apply.Enabled && render(apply.Label) == "Upgrade and publish" && render(cancel.Label) == "Cancel", "buttons %+v %+v", cancel, apply)
	run := as[ApplyManifestUpgrade](t, only(t, m.Update(ConfirmManifestUpgrade{})))
	expect(t, run.Revision == "r1", "applied revision %q", run.Revision)
	expect(t, len(m.Update(ConfirmManifestUpgrade{})) == 0 && len(m.Update(CloseManifestUpgrade{})) == 0 && m.Dialog() != nil, "applying must ignore confirm and close")

	applied := ManifestUpgradeApplied{ManifestUpgradePreview: upgradeFixture(), Sync: &AgentSyncReport{Changed: 1}}
	applied.Publication = AgentPublication{State: PublishPublished, Mode: "direct", Branch: "main", Commit: "0123456789abcdef"}
	m.Update(ManifestUpgradeDone{Applied: applied})
	body = bodyText(m.Dialog())
	expect(t, strings.HasPrefix(body, "kitbag.toml uses schema 2 now; you can choose machines.") && strings.Contains(body, "Published upgrade manifest to schema 2 as 0123456789ab."), "result %q", body)
	cancel, apply = upgradeButtons(t, m)
	expect(t, !apply.Enabled && render(cancel.Label) == "Close", "after apply %+v %+v", cancel, apply)
	m.Update(CloseManifestUpgrade{})
	expect(t, m.Dialog() == nil, "close")
}

func TestManifestUpgradeConflictAndRefusal(t *testing.T) {
	t.Parallel()
	m := NewManifestUpgrade()
	preview := as[PreviewManifestUpgrade](t, only(t, m.Update(OpenManifestUpgrade{})))
	m.Update(ManifestUpgradePreviewed{Seq: preview.Seq, Preview: upgradeFixture()})
	m.Update(ConfirmManifestUpgrade{})
	again := as[PreviewManifestUpgrade](t, only(t, m.Update(ManifestUpgradeDone{Err: &Error{Kind: ErrConflict}})))
	expect(t, again.Seq > preview.Seq && strings.Contains(bodyText(m.Dialog()), "changed since the preview"), "stale %q", bodyText(m.Dialog()))
	m.Update(ManifestUpgradePreviewed{Seq: again.Seq, Preview: upgradeFixture()})
	m.Update(ConfirmManifestUpgrade{})
	m.Update(ManifestUpgradeDone{Err: &Error{Kind: ErrToolTooOld, Detail: "update this machine first"}})
	body := bodyText(m.Dialog())
	_, apply := upgradeButtons(t, m)
	expect(t, strings.Contains(body, ": update this machine first") && apply.Enabled, "refusal %q", body)

	m.Update(OpenManifestUpgrade{})
	current := upgradeFixture()
	current.Current = true
	m.Update(ManifestUpgradePreviewed{Seq: m.issued, Preview: current})
	_, apply = upgradeButtons(t, m)
	expect(t, !apply.Enabled && bodyText(m.Dialog()) == "kitbag.toml already uses schema 2; nothing to do.", "current %q", bodyText(m.Dialog()))
}

// TestAgentsOfferTheUpgradeOnSchema1 shows the hint and the action only on
// schema 1, and reloads after a successful upgrade.
func TestAgentsOfferTheUpgradeOnSchema1(t *testing.T) {
	t.Parallel()
	m := NewAgents()
	snapshot := agentFixture()
	snapshot.Schema = 1
	m.Init()
	m.Update(AgentsLoaded{Seq: 1, Snapshot: snapshot})
	v := m.View()
	last := v.Actions[len(v.Actions)-1]
	_, opens := last.Intent.(OpenManifestUpgrade)
	expect(t, opens && last.Enabled && render(last.Label) == "Upgrade the manifest", "action %+v", last)
	hosts := ""
	for _, row := range v.Detail.Rows {
		if render(row.Label) == "Hosts" {
			hosts = render(row.Value)
		}
	}
	expect(t, hosts == "all hosts; choosing machines needs manifest schema 2", "hint %q", hosts)
	expect(t, len(m.Update(ManifestUpgradeDone{Err: &Error{Kind: ErrInvalid}})) == 0, "failed upgrade reloads")
	as[LoadAgents](t, only(t, m.Update(ManifestUpgradeDone{})))

	snapshot.Schema = 2
	m.Update(AgentsLoaded{Seq: m.issued, Snapshot: snapshot})
	for _, action := range m.View().Actions {
		_, opens := action.Intent.(OpenManifestUpgrade)
		expect(t, !opens, "schema 2 offers the upgrade")
	}
}
