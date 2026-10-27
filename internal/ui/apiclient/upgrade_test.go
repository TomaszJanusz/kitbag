package apiclient

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// TestHostsNeedADaemonThatHonorsThem refuses a preview from a daemon that
// dropped the unknown hosts fields (older than per-host instances), so the
// all-hosts plan it made can never be applied.
func TestHostsNeedADaemonThatHonorsThem(t *testing.T) {
	t.Parallel()
	ignored := workflow.AgentResult{AgentChangeSchema: 1, Plan: agents.Plan{Change: agents.Change{Op: agents.Edit, Target: "claude", Name: "second"}, Revision: "r1"}}
	honored := ignored
	honored.Plan.Change.Hosts = []string{"mac", "ci"}
	widened := ignored
	widened.Plan.Change.AllHosts = true
	for _, tc := range []struct {
		name   string
		change model.AgentChange
		result workflow.AgentResult
		ok     bool
	}{
		{"hosts dropped", model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", Hosts: "mac, ci"}, ignored, false},
		{"all hosts dropped", model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", AllHosts: true}, ignored, false},
		{"hosts honored", model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", Hosts: "mac, ci"}, honored, true},
		{"all hosts honored", model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", AllHosts: true}, widened, true},
		{"no hosts asked", model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second"}, ignored, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transport := &fakeTransport{results: map[string]any{"agents.edit": tc.result}}
			preview, err := Client{Transport: transport}.PreviewAgentChange(t.Context(), tc.change)
			if tc.ok {
				expect(t, err == nil && preview.Change.Hosts == "mac,ci" == (len(tc.result.Plan.Change.Hosts) > 0), "preview %+v %v", preview, err)
			} else {
				expect(t, kindOf(t, err) == model.ErrInvalid, "%s: %v", tc.name, err)
			}
			sent := transport.calls[0].params.Agent
			expect(t, len(sent.Hosts) == len(model.SplitHosts(tc.change.Hosts)) && sent.AllHosts == tc.change.AllHosts, "sent %+v", sent)
		})
	}
}

func TestManifestUpgradeCalls(t *testing.T) {
	t.Parallel()
	result := workflow.UpgradeResult{
		ManifestUpgradeSchema: 1, DryRun: true,
		Upgrade:     manifestupgrade.Plan{Revision: "r1", Title: manifestupgrade.Title, MinToolVersion: "0.1.4", Conversions: []string{"set schema = 2"}, Diff: []string{"+schema = 2"}, Warnings: []string{"w"}, Hosts: []string{"mac"}, SchemaBefore: 1},
		Publication: workflow.Publication{State: workflow.PublishPreview, Mode: "direct", Branch: "main", Next: []string{}},
	}
	applied := result
	applied.DryRun, applied.Publication.State, applied.Publication.Commit = false, workflow.Published, "c1"
	applied.Sync = &workflow.AgentSync{Changed: 1}
	transport := &fakeTransport{results: map[string]any{"manifest.upgrade": result, "stream:manifest.upgrade": applied}}
	c := Client{Transport: transport}
	preview, err := c.PreviewManifestUpgrade(t.Context())
	expect(t, err == nil && preview.Revision == "r1" && preview.CheckedHosts[0] == "mac" && preview.Conversions[0] == "set schema = 2" && preview.Publication.Branch == "main", "preview %+v %v", preview, err)
	done, err := c.ApplyManifestUpgrade(t.Context(), "r1")
	expect(t, err == nil && done.Publication.State == model.PublishPublished && done.Sync.Changed == 1, "applied %+v %v", done, err)
	expect(t, transport.calls[0].params.DryRun && transport.calls[1].method == "stream:manifest.upgrade" && transport.calls[1].params.Revision == "r1", "calls %+v", transport.calls)

	tooOld := &api.Error{Code: api.OperationFailed, Message: "operation failed", Data: api.ErrorData{ExitCode: apply.ToolTooOld, Detail: "update this machine first"}}
	refused := &fakeTransport{errs: map[string]error{"stream:manifest.upgrade": tooOld}}
	done, err = Client{Transport: refused}.ApplyManifestUpgrade(t.Context(), "r1")
	expect(t, kindOf(t, err) == model.ErrToolTooOld && done.ExitCode == int(apply.ToolTooOld), "too old %+v %v", done, err)
}
