package apiclient

import (
	"context"
	"errors"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// PreviewManifestUpgrade plans the upgrade with manifest.upgrade and
// dry_run; nothing is written.
func (c Client) PreviewManifestUpgrade(ctx context.Context) (model.ManifestUpgradePreview, error) {
	result, err := c.upgradeManifest(ctx, workflow.AgentOptions{DryRun: true})
	return upgradePreview(result), err
}

// ApplyManifestUpgrade publishes the upgrade when kitbag.toml is still at
// revision, without the ordinary call deadline: it pushes and syncs.
func (c Client) ApplyManifestUpgrade(ctx context.Context, revision string) (model.ManifestUpgradeApplied, error) {
	result, err := c.upgradeManifest(ctx, workflow.AgentOptions{Revision: revision})
	applied := model.ManifestUpgradeApplied{ManifestUpgradePreview: upgradePreview(result), ExitCode: int(max(result.Code(), apply.ErrorCode(err)))}
	if result.Sync != nil {
		applied.Sync = &model.AgentSyncReport{ExitCode: int(result.Sync.ExitCode), Changed: result.Sync.Changed, Warnings: result.Sync.Warnings}
	}
	var remote *api.Error
	if errors.As(err, &remote) {
		applied.ExitCode = int(remote.ExitCode())
	}
	return applied, err
}

// upgradeManifest calls manifest.upgrade, or runs the same workflow
// in-process without a daemon or with one that predates the method.
func (c Client) upgradeManifest(ctx context.Context, opts workflow.AgentOptions) (workflow.UpgradeResult, error) {
	p := c.params()
	p.DryRun, p.Revision = opts.DryRun, opts.Revision
	var result workflow.UpgradeResult
	call := c.Transport.Stream
	if opts.DryRun {
		call = c.Transport.Call
	}
	err := call(ctx, "manifest.upgrade", p, &result, nil)
	if local(err) && c.Local.Workflow != nil {
		var service workflow.Service
		if service, err = c.Local.Workflow(ctx); err == nil {
			result, err = service.UpgradeManifest(ctx, opts)
		}
	}
	return result, classify(err)
}

func upgradePreview(r workflow.UpgradeResult) model.ManifestUpgradePreview {
	u, p := r.Upgrade, r.Publication
	return model.ManifestUpgradePreview{
		Revision: u.Revision, Title: u.Title, MinToolVersion: u.MinToolVersion,
		Conversions: u.Conversions, Diff: u.Diff, Warnings: u.Warnings, CheckedHosts: u.Hosts,
		Publication:  model.AgentPublication{State: model.PublishState(p.State), Mode: p.Mode, Branch: p.Branch, Commit: p.Commit, Reason: p.Reason, Next: p.Next, Attempts: p.Attempts},
		SchemaBefore: u.SchemaBefore, Current: u.Current,
	}
}
