package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestupgrade"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

const manifestUpgradeLong = `Convert the shared kitbag.toml from schema 1 to schema 2 without losing
anything: schema becomes 2, min_tool_version becomes at least ` + manifestupgrade.FirstSchema2Release + ` (the first
release that reads schema 2), and each [hosts.<h>.targets.<t>.instances] table
that replaces a target's instances becomes the target-level hosts list plus
host patches. Comments, order and layout stay; the result is checked to give
every host the file names, and one it does not, the same active instances.
Layouts that cannot be converted exactly are refused with a hint to edit by
hand. kitbag config and config.toml are not affected.

Every machine that syncs the repository must run kitbag ` + manifestupgrade.FirstSchema2Release + ` or later
BEFORE the upgrade is published: older releases stop syncing (exit 4, or exit 2
for 0.1.0) and apply nothing until they are updated.

Publication works like kitbag agent add: the working copy must be clean, on its
configured branch and without unpublished commits; KitBag fast-forwards it,
commits the change as "` + manifestupgrade.Title + `" and, when this
host's publish mode is direct, pushes without force, then synchronizes. With
publish mode pr nothing is written. A preview is shown first; without a
terminal pass --yes, or --dry-run to only preview.`

type manifestUpgradeFlags struct{ asJSON, dryRun, yes bool }

func (c *commandLine) manifestCommand() *cobra.Command {
	group := &cobra.Command{Use: "manifest", Short: "Operate on the shared kitbag.toml"}
	var flags manifestUpgradeFlags
	upgrade := &cobra.Command{Use: "upgrade", Short: "Convert kitbag.toml to manifest schema 2 and publish it", Long: manifestUpgradeLong, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return c.runManifestUpgrade(cmd, flags)
	}}
	upgrade.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Show the conversions, diff and commit message without writing")
	upgrade.Flags().BoolVar(&flags.asJSON, "json", false, "Print stable manifest_upgrade_schema=1 JSON (needs --yes or --dry-run)")
	upgrade.Flags().BoolVar(&flags.yes, "yes", false, "Apply without asking for confirmation")
	group.AddCommand(upgrade)
	return group
}

func (c *commandLine) runManifestUpgrade(cmd *cobra.Command, flags manifestUpgradeFlags) error {
	switch {
	case c.flags.host != "":
		return &commandError{err: errors.New("--host is not supported by manifest upgrade: the manifest is shared by every host"), code: apply.Usage}
	case flags.asJSON && !flags.yes && !flags.dryRun:
		return &commandError{err: errors.New("--json needs --yes or --dry-run"), code: apply.Usage}
	}
	service, err := c.service(cmd.Context())
	if err != nil {
		return classified(err)
	}
	if flags.yes || flags.dryRun {
		result, runErr := service.UpgradeManifest(cmd.Context(), workflow.AgentOptions{DryRun: flags.dryRun})
		return c.finishUpgrade(result, runErr, flags.asJSON)
	}
	if !c.host.IsTerminal(0) {
		return &commandError{err: fmt.Errorf("confirming the manifest upgrade needs a terminal; pass --yes or --dry-run: %w", fs.ErrInvalid), code: apply.Usage}
	}
	return c.confirmUpgrade(cmd, service)
}

// confirmUpgrade previews, asks and applies the previewed revision, showing
// a new preview when kitbag.toml moved upstream in between.
func (c *commandLine) confirmUpgrade(cmd *cobra.Command, service workflow.Service) error {
	var err error
	scanner := bufio.NewScanner(cmd.InOrStdin())
	for range previewRounds {
		preview, previewErr := service.UpgradeManifest(cmd.Context(), workflow.AgentOptions{DryRun: true})
		if previewErr != nil || preview.Upgrade.Current {
			return c.finishUpgrade(preview, previewErr, false)
		}
		if err = c.printUpgradePreview(preview); err != nil {
			return classified(err)
		}
		ok, askErr := ask(c.stderr, scanner, "Upgrade kitbag.toml to schema 2? [y/N] ")
		if askErr != nil || !ok {
			return classified(askErr)
		}
		result, runErr := service.UpgradeManifest(cmd.Context(), workflow.AgentOptions{Revision: preview.Upgrade.Revision})
		if !errors.Is(runErr, apply.ErrConflict) || result.Publication.State == workflow.Superseded {
			return c.finishUpgrade(result, runErr, false)
		}
		if _, err = fmt.Fprintln(c.stderr, "kitbag.toml changed upstream since this preview; here is the new one."); err != nil {
			return classified(err)
		}
	}
	return &commandError{err: fmt.Errorf("kitbag.toml keeps changing upstream; try again later: %w", apply.ErrConflict), code: apply.Conflict}
}

func (c *commandLine) finishUpgrade(result workflow.UpgradeResult, err error, asJSON bool) error {
	c.code = max(c.code, result.Code())
	var printErr error
	switch {
	case asJSON && (err == nil || result.Publication.State != ""):
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		printErr = enc.Encode(result)
	case err == nil && result.Upgrade.Current:
		_, printErr = fmt.Fprintf(c.stdout, "%s already uses manifest schema %d; nothing to do.\n", result.Manifest, result.Upgrade.SchemaBefore)
	case result.DryRun && err == nil:
		printErr = c.printUpgradePreview(result)
	case result.Publication.State != "":
		printErr = c.printUpgradeResult(result)
	}
	if printErr != nil {
		return classified(printErr)
	}
	return classified(err)
}

func (c *commandLine) printUpgradePreview(r workflow.UpgradeResult) error {
	u := r.Upgrade
	// The heading, manifest, checked hosts, commit and route lines frame them.
	const frame = 5
	lines := make([]string, 0, len(u.Conversions)+len(u.Diff)+frame)
	lines = append(lines, fmt.Sprintf("Upgrade: manifest schema %d to 2", u.SchemaBefore), "Manifest: "+r.Manifest)
	for _, conversion := range u.Conversions {
		lines = append(lines, "  - "+conversion)
	}
	lines = append(lines, u.Diff...)
	lines = append(lines, "Checked hosts: "+strings.Join(u.Hosts, ", ")+" (same active instances before and after)", "Commit: "+u.Title)
	route := "publish: direct push to " + r.Publication.Branch
	if r.Publication.Mode != "direct" {
		route = "publish: mode " + r.Publication.Mode + " (nothing is written; you get the commands to propose it)"
	}
	lines = append(lines, route)
	if _, err := fmt.Fprintln(c.stdout, strings.Join(lines, "\n")); err != nil {
		return err
	}
	return printWarnings(c.stderr, u.Warnings)
}

func (c *commandLine) printUpgradeResult(r workflow.UpgradeResult) error {
	if r.Publication.State == workflow.Manual {
		if err := c.printUpgradePreview(r); err != nil {
			return err
		}
	}
	shim := workflow.AgentResult{Plan: agents.Plan{Title: r.Upgrade.Title}, Publication: r.Publication}
	lines := append([]string{publicationLine(shim)}, r.Publication.Next...)
	for i := 1; i < len(lines); i++ {
		lines[i] = "  " + lines[i]
	}
	if _, err := fmt.Fprintln(c.stdout, strings.Join(lines, "\n")); err != nil {
		return err
	}
	if r.Publication.State == workflow.Published && r.SyncResult != nil {
		if err := c.printSync(*r.SyncResult); err != nil {
			return err
		}
	}
	if r.Publication.State == workflow.Manual {
		return nil
	}
	return printWarnings(c.stderr, r.Upgrade.Warnings)
}
