package apiclient

import (
	"context"
	"errors"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// local reports whether err permits the in-process path: no daemon listens,
// or an older daemon does not know the method (API 1.0).
func local(err error) bool {
	var remote *api.Error
	return noDaemon(err) || (errors.As(err, &remote) && remote.Code == api.MethodNotFound)
}

// Agents reads agents.list with this process's relocation variables (only
// AgentVariables are sent), falling back to workflow.Service.Agents.
func (c Client) Agents(ctx context.Context, probe bool) (model.AgentsSnapshot, error) {
	p := c.params()
	p.Probe, p.Env = probe, c.agentEnv()
	var result discover.Snapshot
	err := c.Transport.Call(ctx, "agents.list", p, &result, nil)
	daemon := true
	if local(err) && c.Local.Workflow != nil {
		daemon = false
		var service workflow.Service
		if service, err = c.Local.Workflow(ctx); err == nil {
			result, err = service.Agents(probe)
		}
	}
	if err != nil {
		return model.AgentsSnapshot{Daemon: daemon}, classify(err)
	}
	return agentsSnapshot(result, daemon), nil
}

func (c Client) agentEnv() map[string]string {
	env := map[string]string{}
	if c.Env == nil {
		return env
	}
	for _, key := range c.AgentVariables {
		if value, ok := c.Env.LookupEnv(key); ok {
			env[key] = value
		}
	}
	return env
}

// PreviewAgentChange plans change with agents.<op> and dry_run; nothing is
// written.
func (c Client) PreviewAgentChange(ctx context.Context, change model.AgentChange) (model.AgentPreview, error) {
	result, err := c.changeAgent(ctx, change, workflow.AgentOptions{DryRun: true})
	return agentPreview(result), err
}

// ApplyAgentChange publishes change when the manifest is still at revision,
// without the ordinary call deadline: it pushes and syncs.
func (c Client) ApplyAgentChange(ctx context.Context, change model.AgentChange, revision string) (model.AgentApplied, error) {
	result, err := c.changeAgent(ctx, change, workflow.AgentOptions{Revision: revision})
	applied := model.AgentApplied{AgentPreview: agentPreview(result), ExitCode: int(max(result.Code(), apply.ErrorCode(err)))}
	if result.Released != nil {
		applied.Released = &model.AgentRelease{Dir: result.Released.Dir, Changed: result.Released.Changed, Warnings: result.Released.Warnings}
	}
	if result.Sync != nil {
		applied.Sync = &model.AgentSyncReport{ExitCode: int(result.Sync.ExitCode), Changed: result.Sync.Changed, Warnings: result.Sync.Warnings}
	}
	var remote *api.Error
	if errors.As(err, &remote) {
		applied.ExitCode = int(remote.ExitCode())
	}
	return applied, err
}

func (c Client) changeAgent(ctx context.Context, change model.AgentChange, opts workflow.AgentOptions) (workflow.AgentResult, error) {
	core := agents.Change{Op: agents.Op(change.Op), Kind: change.Kind, Target: change.Target, Name: change.Name, Dir: change.Dir, Settings: change.Settings}
	p := c.params()
	p.Agent, p.DryRun, p.Revision = &core, opts.DryRun, opts.Revision
	var result workflow.AgentResult
	call := c.Transport.Stream
	if opts.DryRun {
		call = c.Transport.Call
	}
	err := call(ctx, "agents."+string(change.Op), p, &result, nil)
	if local(err) && c.Local.Workflow != nil {
		var service workflow.Service
		if service, err = c.Local.Workflow(ctx); err == nil {
			result, err = service.ChangeAgent(ctx, core, opts)
		}
	}
	return result, classify(err)
}

func agentsSnapshot(s discover.Snapshot, daemon bool) model.AgentsSnapshot {
	snapshot := model.AgentsSnapshot{Host: s.Host, KnownHosts: s.KnownHosts, Kinds: s.Kinds, Warnings: s.Warnings, Schema: s.Schema, Daemon: daemon}
	for _, c := range s.Candidates {
		snapshot.Candidates = append(snapshot.Candidates, candidate(c))
	}
	for _, i := range s.Instances {
		snapshot.Instances = append(snapshot.Instances, model.AgentInstance{
			AgentCandidate: candidate(i.ListedCandidate), Target: i.Target, Name: i.Name,
			Hosts: i.Hosts, ExcludedHosts: i.ExcludedHosts, Implicit: i.Implicit,
			Enabled: i.Enabled, Settings: i.Settings, ActiveHere: i.ActiveHere,
		})
	}
	return snapshot
}

func candidate(c discover.ListedCandidate) model.AgentCandidate {
	return model.AgentCandidate{Kind: c.Kind, Dir: c.Dir, Variable: c.Variable, Source: model.AgentSource(c.Source), Presence: model.AgentPresence(c.Presence)}
}

func agentPreview(r workflow.AgentResult) model.AgentPreview {
	ch := r.Plan.Change
	p := r.Publication
	preview := model.AgentPreview{
		Manifest: r.Manifest, Directory: r.Directory,
		Change:      model.AgentChange{Op: model.AgentOp(ch.Op), Kind: ch.Kind, Target: ch.Target, Name: ch.Name, Dir: ch.Dir, Settings: ch.Settings},
		Publication: model.AgentPublication{State: model.PublishState(p.State), Mode: p.Mode, Branch: p.Branch, Commit: p.Commit, Reason: p.Reason, Next: p.Next, Attempts: p.Attempts},
		Revision:    r.Plan.Revision, Title: r.Plan.Title, Message: r.Plan.Message, Dropped: r.Plan.Dropped,
		Diff: r.Plan.Diff, Warnings: r.Plan.Warnings,
	}
	if r.Effects != nil {
		preview.Effects = make([]model.AgentEffect, 0, len(r.Effects))
	}
	for _, effect := range r.Effects {
		preview.Effects = append(preview.Effects, model.AgentEffect{Path: effect.Path, Kind: effect.Kind, Backup: effect.Backup, Warning: effect.Warning, Existing: effect.Existing})
	}
	return preview
}
