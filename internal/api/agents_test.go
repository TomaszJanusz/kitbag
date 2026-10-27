package api

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

type headOnly struct{ vcs.Backend }

func (headOnly) Head(context.Context, string) (vcs.Head, error) {
	return vcs.Head{Branch: "main", Commit: "abc"}, nil
}

// agentHandlers serves a working copy declaring claude/main in a fake HOME
// whose daemon environment relocates Claude to /daemon/claude.
func agentHandlers(t *testing.T) Handlers {
	t.Helper()
	home := filepath.Join(string(filepath.Separator), "home", "u")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "KITBAG_HOST": "test", "CLAUDE_CONFIG_DIR": "/daemon/claude"}))
	repo := filepath.Join(home, "repo")
	for _, dir := range []string{repo, filepath.Join(home, ".claude"), filepath.Join(home, ".claude_2")} {
		if err := h.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := "schema=1\n[targets.claude]\nagent='claude'\n[targets.claude.instances.main]\ndir='~/.claude'\n"
	if err := h.WriteAtomic(filepath.Join(repo, "kitbag.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	kinds := discover.Table{{Agent: "claude", ProbePrefix: ".claude", Variable: "CLAUDE_CONFIG_DIR", DefaultDir: "~/.claude", Home: func(env platform.Env) (string, error) {
		return env.AgentHome("CLAUDE_CONFIG_DIR", ".claude")
	}}}
	service := workflow.Service{Host: h, Backend: headOnly{}, Repository: repo, Version: "0.1.0", Kinds: kinds, Now: time.Now}
	return Handlers{Host: h, Events: NewEvents(), Service: func(context.Context, Options) (workflow.Service, error) { return service, nil }}
}

func listDefault(t *testing.T, h Handlers, env map[string]string) discover.ListedCandidate {
	t.Helper()
	data, problem := h.call(t.Context(), "agents.list", Params{Env: env}, nil, func(Event) {})
	if problem != nil {
		t.Fatal(problem)
	}
	snapshot, ok := data.(discover.Snapshot)
	if !ok {
		t.Fatalf("agents.list returned %T", data)
	}
	for _, c := range snapshot.Candidates {
		if c.Source == discover.Default || c.Source == discover.Environment {
			return c
		}
	}
	return discover.ListedCandidate{}
}

func TestAgentsListUsesTheCallersRelocation(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	if c := listDefault(t, h, nil); c.Dir != "/daemon/claude" {
		t.Fatalf("without env the daemon's variables apply: %+v", c)
	}
	if c := listDefault(t, h, map[string]string{}); c.Source != "" {
		t.Fatalf("an unset caller variable selects the declared default, already listed: %+v", c)
	}
	c := listDefault(t, h, map[string]string{"CLAUDE_CONFIG_DIR": "/home/u/.claude_2", "UNRELATED": "/x"})
	if c.Dir != "/home/u/.claude_2" || c.Source != discover.Environment || c.Variable != "CLAUDE_CONFIG_DIR" {
		t.Fatalf("caller relocation: %+v", c)
	}
}

func TestAgentChangePreview(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	data, problem := h.call(t.Context(), "agents.add", Params{DryRun: true, Agent: &agents.Change{Kind: "claude", Name: "second", Dir: "~/.claude_2"}}, nil, func(Event) {})
	if problem != nil {
		t.Fatal(problem)
	}
	result, ok := data.(workflow.AgentResult)
	if !ok || result.Publication.State != workflow.PublishPreview || result.Plan.Change.Op != agents.Add || result.Publication.Branch != "main" {
		t.Fatalf("preview: %+v", data)
	}
}

func TestAgentChangeRefusalsCarryDetail(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	_, problem := h.call(t.Context(), "agents.add", Params{DryRun: true, Agent: &agents.Change{Kind: "claude", Name: "Bad", Dir: "~/x"}}, nil, func(Event) {})
	if problem == nil || problem.Data.ExitCode != apply.Usage || !strings.Contains(problem.Data.Detail, "instance name") || !strings.Contains(problem.Error(), "instance name") {
		t.Fatalf("validation detail: %+v", problem)
	}
	_, problem = h.call(t.Context(), "agents.remove", Params{DryRun: true, Revision: strings.Repeat("0", 64), Agent: &agents.Change{Target: "claude", Name: "main"}}, nil, func(Event) {})
	if problem == nil || problem.Data.ExitCode != apply.Conflict || !strings.Contains(problem.Data.Detail, "changed since the preview") {
		t.Fatalf("stale revision: %+v", problem)
	}
	_, problem = h.call(t.Context(), "agents.edit", Params{}, nil, func(Event) {})
	if problem == nil || problem.Code != InvalidParams {
		t.Fatalf("missing agent: %+v", problem)
	}
}

func TestAgentErrorHidesOtherDiagnostics(t *testing.T) {
	t.Parallel()
	if problem := agentError(platform.ErrLocked); problem.Code != Busy || problem.Data.Detail != "" {
		t.Fatalf("busy: %+v", problem)
	}
	if problem := agentError(context.DeadlineExceeded); problem.Data.Detail != "" || problem.Error() != "operation failed" {
		t.Fatalf("operational error leaked detail: %+v", problem)
	}
	if agentError(nil) != nil {
		t.Fatal("nil error")
	}
}

// TestManifestUpgrade previews the schema-1 fixture, which this tool (0.1.0)
// could not read after the upgrade, so the preview warns.
func TestManifestUpgrade(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	data, problem := h.call(t.Context(), "manifest.upgrade", Params{DryRun: true}, nil, func(Event) {})
	if problem != nil {
		t.Fatal(problem)
	}
	preview, ok := data.(workflow.UpgradeResult)
	if !ok || preview.ManifestUpgradeSchema != 1 || preview.Upgrade.SchemaBefore != 1 || preview.Upgrade.Current || preview.Publication.State != workflow.PublishPreview {
		t.Fatalf("preview: %+v", data)
	}
	if !slices.Contains(preview.Upgrade.Conversions, "set schema = 2") || !strings.Contains(strings.Join(preview.Upgrade.Warnings, "\n"), "this kitbag (0.1.0) could not read the upgraded manifest") {
		t.Fatalf("conversions %q warnings %q", preview.Upgrade.Conversions, preview.Upgrade.Warnings)
	}
	if !slices.Contains(methods(), "manifest.upgrade") {
		t.Fatal("version.get does not list manifest.upgrade")
	}
}

// TestUpgradeErrorDetail shows why publishing was refused, and nothing for
// operational errors.
func TestUpgradeErrorDetail(t *testing.T) {
	t.Parallel()
	tooOld := fmt.Errorf("this kitbag (0.1.0) could not read the upgraded manifest; update this machine first: %w", &manifest.VersionError{Required: "0.1.4", Current: "0.1.0"})
	if problem := upgradeError(tooOld); problem == nil || problem.Data.ExitCode != apply.ToolTooOld || !strings.Contains(problem.Data.Detail, "update this machine first") {
		t.Fatalf("too old: %+v", problem)
	}
	if problem := upgradeError(context.DeadlineExceeded); problem.Data.Detail != "" {
		t.Fatalf("operational error leaked detail: %+v", problem)
	}
}

func TestManifestUpgradeRefusalsCarryDetail(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	service, err := h.Service(t.Context(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	ambiguous := "schema=1\n[targets.claude]\nagent='claude'\n[targets.claude.instances.main]\ndir='~/.claude'\n[hosts.mac.targets.claude.instances.other]\ndir='~/.claude'\n"
	if err = service.Host.WriteAtomic(filepath.Join(service.Repository, "kitbag.toml"), []byte(ambiguous), 0o600); err != nil {
		t.Fatal(err)
	}
	_, problem := h.call(t.Context(), "manifest.upgrade", Params{DryRun: true}, nil, func(Event) {})
	if problem == nil || problem.Data.ExitCode != apply.Usage || !strings.Contains(problem.Data.Detail, "name the same directory ~/.claude") {
		t.Fatalf("refused layout: %+v", problem)
	}
}

func TestAgentHostsNeedSchema2(t *testing.T) {
	t.Parallel()
	h := agentHandlers(t)
	_, problem := h.call(t.Context(), "agents.edit", Params{DryRun: true, Agent: &agents.Change{Target: "claude", Name: "main", Hosts: []string{"mac"}}}, nil, func(Event) {})
	if problem == nil || problem.Data.ExitCode != apply.Usage || !strings.Contains(problem.Data.Detail, "kitbag manifest upgrade --dry-run") {
		t.Fatalf("schema-1 hosts: %+v", problem)
	}
}
