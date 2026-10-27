package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// errCurrent stops an upgrade of a manifest that already uses schema 2.
var errCurrent = errors.New("manifest already uses the latest schema")

// UpgradeResult is the manifest_upgrade_schema=1 report of kitbag manifest
// upgrade. Upgrade.Current means there was nothing to do.
type UpgradeResult struct {
	Sync                  *AgentSync           `json:"sync"`
	SyncResult            *SyncResult          `json:"-"`
	Manifest              string               `json:"manifest"`
	RevisionAfter         string               `json:"revision_after,omitempty"`
	Publication           Publication          `json:"publication"`
	Upgrade               manifestupgrade.Plan `json:"upgrade"`
	ManifestUpgradeSchema int                  `json:"manifest_upgrade_schema"`
	code                  apply.Code
	DryRun                bool `json:"dry_run"`
}

// Code is the exit classification, as for agent changes.
func (r UpgradeResult) Code() apply.Code { return r.code }

// UpgradeManifest converts kitbag.toml from schema 1 to 2 and publishes it
// like ChangeAgent publishes an agent change (ADR 12): previews write
// nothing; otherwise, under the state lock and on a clean, fast-forwarded
// working copy, it commits and, in direct mode, pushes without force, then
// synchronizes. In pr mode nothing is written.
func (s Service) UpgradeManifest(ctx context.Context, opts AgentOptions) (UpgradeResult, error) {
	var upgrade manifestupgrade.Plan
	change := manifestChange{
		prepare: func(data []byte, host string) (agents.Plan, error) {
			var err error
			upgrade, err = manifestupgrade.Prepare(manifestupgrade.Input{Env: s.Host, Kinds: s.Kinds, Filename: s.manifestPath(), HostName: host, Manifest: data})
			if err != nil {
				return agents.Plan{}, err
			}
			if upgrade.Current {
				return agents.Plan{}, errCurrent
			}
			if err = s.readsUpgrade(&upgrade, opts.DryRun); err != nil {
				return agents.Plan{}, err
			}
			return agents.Plan{Revision: upgrade.Revision, Title: upgrade.Title, Message: upgrade.Message, Diff: upgrade.Diff, Warnings: upgrade.Warnings, After: upgrade.After}, nil
		},
		branch: func(agents.Plan) string { return "kitbag/manifest-schema-2" },
	}
	inner, err := s.runChange(ctx, change, opts)
	result := UpgradeResult{
		Sync: inner.Sync, SyncResult: inner.SyncResult, Manifest: inner.Manifest, RevisionAfter: inner.RevisionAfter,
		Upgrade: upgrade, Publication: inner.Publication, ManifestUpgradeSchema: 1, code: inner.Code(), DryRun: opts.DryRun,
	}
	if errors.Is(err, errCurrent) {
		result.Publication.State = ""
		return result, nil
	}
	return result, err
}

// readsUpgrade refuses to publish a manifest this kitbag could not read
// afterwards (for example a development build), which would leave this
// machine unable to sync; a preview only warns.
func (s Service) readsUpgrade(upgrade *manifestupgrade.Plan, dryRun bool) error {
	err := (&manifest.Manifest{MinToolVersion: upgrade.MinToolVersion}).CheckVersion(s.Version)
	if err == nil {
		return nil
	}
	message := fmt.Sprintf("this kitbag (%s) could not read the upgraded manifest, which requires kitbag %s; update this machine first", s.Version, upgrade.MinToolVersion)
	if dryRun {
		upgrade.Warnings = append(upgrade.Warnings, message)
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
