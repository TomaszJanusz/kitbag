package apiclient

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

func TestAgentsForwardsOnlyAgentVariables(t *testing.T) {
	t.Parallel()
	snapshot := discover.Snapshot{
		AgentsSchema: 1, Host: "mac", Kinds: []string{"claude"}, Schema: 1,
		Instances:  []discover.Instance{{ListedCandidate: discover.ListedCandidate{Kind: "claude", Dir: "/h/.claude", Source: discover.Declared, Presence: discover.Present}, Target: "claude", Name: "main", Hosts: []string{"mac", "ci"}, Enabled: true, ActiveHere: true}},
		Candidates: []discover.ListedCandidate{{Kind: "claude", Dir: "/h/.claude_2", Source: discover.Environment, Variable: "CLAUDE_CONFIG_DIR", Presence: discover.Missing}},
	}
	transport := &fakeTransport{results: map[string]any{"agents.list": snapshot}}
	env := fake.New(fake.WithEnv(map[string]string{"CLAUDE_CONFIG_DIR": "/h/.claude_2", "SECRET_TOKEN": "x"}))
	got, err := Client{Transport: transport, Env: env, AgentVariables: []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"}}.Agents(t.Context(), true)
	expect(t, err == nil && got.Daemon && got.Host == "mac" && got.Schema == 1, "snapshot %+v %v", got, err)
	expect(t, len(got.Instances) == 1 && got.Instances[0].Name == "main" && got.Instances[0].Presence == model.PresencePresent, "instances %+v", got.Instances)
	expect(t, strings.Join(got.Instances[0].Hosts, ",") == "mac,ci", "hosts %+v", got.Instances[0].Hosts)
	expect(t, len(got.Candidates) == 1 && got.Candidates[0].Source == model.AgentSourceEnvironment && got.Candidates[0].Variable == "CLAUDE_CONFIG_DIR", "candidates %+v", got.Candidates)
	p := transport.calls[0].params
	expect(t, p.Probe && len(p.Env) == 1 && p.Env["CLAUDE_CONFIG_DIR"] == "/h/.claude_2", "params %+v", p)
}

// TestAgentsFallBackForOldDaemons proves both fallbacks: no daemon, and a
// 1.0 daemon without agents.* (method not found).
func TestAgentsFallBackForOldDaemons(t *testing.T) {
	t.Parallel()
	notFound := &api.Error{Code: api.MethodNotFound, Message: "method not found", Data: api.ErrorData{ExitCode: apply.Usage}}
	local := Local{Workflow: func(context.Context) (workflow.Service, error) { return workflow.Service{}, fs.ErrInvalid }}
	for name, err := range map[string]error{"no daemon": platform.ErrNoDaemon, "old daemon": notFound} {
		transport := &fakeTransport{errs: map[string]error{"agents.list": err, "agents.add": err}}
		_, listErr := Client{Transport: transport, Local: local}.Agents(t.Context(), false)
		expect(t, kindOf(t, listErr) == model.ErrInvalid, "%s list: %v", name, listErr)
		_, addErr := Client{Transport: transport, Local: local}.PreviewAgentChange(t.Context(), model.AgentChange{Op: model.AgentAdd})
		expect(t, kindOf(t, addErr) == model.ErrInvalid, "%s preview: %v", name, addErr)
	}
	busy := &fakeTransport{errs: map[string]error{"agents.list": &api.Error{Code: api.Busy, Message: "busy", Data: api.ErrorData{ExitCode: apply.Warnings}}}}
	_, err := Client{Transport: busy, Local: local}.Agents(t.Context(), false)
	expect(t, kindOf(t, err) == model.ErrBusy, "busy must not fall back: %v", err)
}

func TestPreviewAndApplyAgentChange(t *testing.T) {
	t.Parallel()
	off := false
	result := workflow.AgentResult{
		Manifest: "/repo/kitbag.toml", Directory: "/home/u/.claude_2", Effects: []workflow.AgentEffect{{Path: "/home/u/.claude_2/settings.json", Kind: "merge", Existing: true}},
		AgentChangeSchema: 1, Plan: agents.Plan{Change: agents.Change{Op: agents.Edit, Kind: "claude", Target: "claude", Name: "second", Dir: "~/.claude_2", Settings: &off, Hosts: []string{"mac", "ci"}}, Revision: "r1", Title: "chore(kitbag): edit claude instance second (settings off)", Diff: []string{"+settings = false"}},
		Publication: workflow.Publication{State: workflow.Published, Mode: "direct", Branch: "main", Commit: "c1", Attempts: 1, Next: []string{}},
		Sync:        &workflow.AgentSync{ExitCode: apply.Warnings, Changed: 2, Warnings: []string{"w"}}, Released: &workflow.AgentRelease{Dir: "~/.claude_2", Changed: []string{"discard x"}, Warnings: []string{}},
	}
	transport := &fakeTransport{results: map[string]any{"agents.edit": result, "stream:agents.edit": result}}
	c := Client{Transport: transport}
	change := model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", Settings: &off, Hosts: "mac, ci"}
	preview, err := c.PreviewAgentChange(t.Context(), change)
	expect(t, err == nil && preview.Revision == "r1" && preview.Diff[0] == "+settings = false", "preview %+v %v", preview, err)
	checkAgentPreviewEffects(t, preview, result)
	expect(t, preview.Change.Settings != nil && !*preview.Change.Settings && preview.Change.Hosts == "mac,ci", "settings %+v", preview.Change)
	applied, err := c.ApplyAgentChange(t.Context(), change, "r1")
	expect(t, err == nil && applied.Publication.State == model.PublishPublished && applied.ExitCode == 1, "applied %+v %v", applied, err)
	expect(t, applied.Sync.Changed == 2 && applied.Released.Dir == "~/.claude_2", "reports %+v", applied)
	checkAgentCalls(t, transport.calls)
}

func checkAgentCalls(t *testing.T, calls []call) {
	t.Helper()
	preview, publish := calls[0], calls[1]
	expect(t, preview.method == "agents.edit" && preview.params.DryRun && preview.params.Agent.Op == agents.Edit, "dry call %+v", preview)
	expect(t, publish.method == "stream:agents.edit" && !publish.params.DryRun && publish.params.Revision == "r1" && strings.Join(preview.params.Agent.Hosts, ",") == "mac,ci" && strings.Join(publish.params.Agent.Hosts, ",") == "mac,ci", "apply call %+v", publish)
}

func TestApplyAgentChangeConflictKeepsDetail(t *testing.T) {
	t.Parallel()
	stale := &api.Error{Code: api.OperationFailed, Message: "operation failed", Data: api.ErrorData{ExitCode: apply.Conflict, Detail: "kitbag.toml changed since the preview"}}
	transport := &fakeTransport{errs: map[string]error{"stream:agents.add": stale}}
	applied, err := Client{Transport: transport}.ApplyAgentChange(t.Context(), model.AgentChange{Op: model.AgentAdd}, "r0")
	var classified *model.Error
	expect(t, errors.As(err, &classified) && classified.Kind == model.ErrConflict && classified.Detail == "kitbag.toml changed since the preview", "error %v", err)
	expect(t, applied.ExitCode == int(apply.Conflict), "exit %d", applied.ExitCode)
}

func checkAgentPreviewEffects(t *testing.T, preview model.AgentPreview, result workflow.AgentResult) {
	t.Helper()
	expect(t, preview.Manifest == result.Manifest && preview.Directory == result.Directory, "paths %+v", preview)
	expect(t, len(preview.Effects) == 1, "effects %+v", preview)
	expect(t, preview.Effects[0].Kind == "merge" && preview.Effects[0].Existing, "effects %+v", preview)
}

// TestHonoredHosts refuses a preview from a daemon that dropped the hosts
// list, which would otherwise plan an all-hosts change.
func TestHonoredHosts(t *testing.T) {
	t.Parallel()
	requested := agents.Change{Hosts: []string{"mac"}}
	if err := honoredHosts(requested, agents.Change{}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("dropped hosts accepted: %v", err)
	}
	if err := honoredHosts(requested, agents.Change{Hosts: []string{"mac"}}); err != nil {
		t.Fatal(err)
	}
	if err := honoredHosts(agents.Change{AllHosts: true}, agents.Change{}); err != nil {
		t.Fatalf("all hosts needs no echo: %v", err)
	}
	if err := honoredHosts(agents.Change{}, agents.Change{Hosts: []string{"mac"}}); err != nil {
		t.Fatalf("kept hosts are fine: %v", err)
	}
}

// TestAgentChangeCarriesHosts converts the model's comma list to the API's
// array and back.
func TestAgentChangeCarriesHosts(t *testing.T) {
	t.Parallel()
	preview := agentPreview(workflow.AgentResult{Plan: agents.Plan{Change: agents.Change{Op: agents.Edit, Hosts: []string{"mac", "ci"}, AllHosts: false}}})
	if preview.Change.Hosts != "mac,ci" || preview.Change.AllHosts {
		t.Fatalf("preview change %+v", preview.Change)
	}
}

// TestPreviewFromDaemonWithoutHosts sends the list as an array and refuses
// a preview whose daemon dropped it; nothing can be applied without one.
func TestPreviewFromDaemonWithoutHosts(t *testing.T) {
	t.Parallel()
	old := workflow.AgentResult{AgentChangeSchema: 1, Plan: agents.Plan{Change: agents.Change{Op: agents.Add, Kind: "claude", Name: "second", Dir: "~/.claude_2"}, Revision: "r1"}}
	transport := &fakeTransport{results: map[string]any{"agents.add": old}}
	_, err := Client{Transport: transport}.PreviewAgentChange(t.Context(), model.AgentChange{Op: model.AgentAdd, Kind: "claude", Name: "second", Dir: "~/.claude_2", Hosts: "mac, ci"})
	expect(t, kindOf(t, err) == model.ErrInvalid && strings.Contains(err.Error(), "cannot choose machines"), "old daemon preview: %v", err)
	expect(t, len(transport.calls) == 1 && strings.Join(transport.calls[0].params.Agent.Hosts, ",") == "mac,ci", "sent %+v", transport.calls)
}

func TestAgentChangeClearsHosts(t *testing.T) {
	t.Parallel()
	result := workflow.AgentResult{Plan: agents.Plan{Change: agents.Change{Op: agents.Edit, AllHosts: true}, Revision: "r1"}}
	transport := &fakeTransport{results: map[string]any{"agents.edit": result, "stream:agents.edit": result}}
	c := Client{Transport: transport}
	change := model.AgentChange{Op: model.AgentEdit, Target: "claude", Name: "second", AllHosts: true}
	preview, err := c.PreviewAgentChange(t.Context(), change)
	expect(t, err == nil && preview.Change.AllHosts && preview.Change.Hosts == "", "preview %+v %v", preview, err)
	_, err = c.ApplyAgentChange(t.Context(), change, preview.Revision)
	expect(t, err == nil && len(transport.calls) == 2, "apply %v calls %+v", err, transport.calls)
	for _, call := range transport.calls {
		expect(t, call.params.Agent.AllHosts && len(call.params.Agent.Hosts) == 0, "scope %+v", call.params.Agent)
	}
}
