package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/adopt"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

// adoptReport is the stable adopt --json document (adopt_schema 1).
type adoptReport struct {
	Adopted     any  `json:"adopted"`
	Moved       any  `json:"moved"`
	Remaining   any  `json:"remaining"`
	Undeclared  any  `json:"undeclared"`
	AdoptSchema int  `json:"adopt_schema"`
	DryRun      bool `json:"dry_run"`
}

func (c *commandLine) adoptCommand() *cobra.Command {
	var asJSON, dryRun, moveRealDirs bool
	cmd := &cobra.Command{
		Use: "adopt", Short: "Take over destinations agent-config-sync already manages",
		Long: "Record ownership of every declared destination that already holds what sync would write: links to the exact source, " +
			"identical files and agent-config-sync's marker-only variants. No file is rewritten, backed up or deleted, so the next sync " +
			"has nothing to do for them; anything else is listed as remaining. Requires migration.legacy_roots in config.toml. " +
			"--move-real-dirs moves real skill directories aside once, as agent-config-sync --adopt did.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service, err := c.service(cmd.Context())
			if err != nil {
				return classified(err)
			}
			result, err := service.Adopt(moveRealDirs, dryRun)
			if err != nil {
				return classified(err)
			}
			c.code = result.Report.Moves.Code()
			return classified(c.printAdopt(result, asJSON))
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would be adopted without saving state")
	cmd.Flags().BoolVar(&moveRealDirs, "move-real-dirs", false, "Move real skill directories at declared destinations aside once")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable adopt_schema=1 JSON")
	return cmd
}

func (c *commandLine) printAdopt(result workflow.AdoptResult, asJSON bool) error {
	report := result.Report
	if err := printWarnings(c.stderr, result.Warnings); err != nil {
		return err
	}
	if err := printApply(io.Discard, c.stderr, report.Moves); err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(adoptReport{AdoptSchema: 1, Adopted: report.Adopted, Moved: report.Moved, Remaining: report.Remaining, Undeclared: report.Undeclared, DryRun: report.DryRun})
	}
	return c.printAdoptText(report)
}

func (c *commandLine) printAdoptText(report adopt.Report) error {
	verb := "adopt"
	if report.DryRun {
		verb = "would adopt"
	}
	for _, a := range report.Adopted {
		line := fmt.Sprintf("%s %s [%s]", verb, a.Path, a.Kind)
		if a.Legacy {
			line += " (agent-config-sync marker kept until the output changes)"
		}
		if _, err := fmt.Fprintln(c.stdout, line); err != nil {
			return err
		}
	}
	var lines []string
	for _, m := range report.Moved {
		lines = append(lines, fmt.Sprintf("move %s -> %s", m.Path, m.Backup))
	}
	for _, path := range report.Undeclared {
		lines = append(lines, "left alone (not declared): "+path)
	}
	if len(report.Remaining) > 0 {
		lines = append(lines, "The next sync would still change:")
	}
	if err := printWarnings(c.stdout, lines); err != nil {
		return err
	}
	if len(report.Remaining) == 0 {
		return nil
	}
	return writeDiff(c.stdout, planOf(report.Remaining), c.colour())
}

func (c *commandLine) unmanageCommand() *cobra.Command {
	var asJSON, dryRun bool
	cmd := &cobra.Command{
		Use: "unmanage", Short: "Remove everything KitBag created and restore what it replaced",
		Long: "Delete unchanged links, generated files and copies that KitBag owns, remove its hook groups from merged settings, " +
			"and rename one-time backups back into place. Changed or unowned entries are left alone and reported as conflicts. " +
			"The steps are journaled; a fresh sync manages the machine again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := jsonNeedsDryRun(asJSON, dryRun); err != nil {
				return err
			}
			service, err := c.service(cmd.Context())
			if err != nil {
				return classified(err)
			}
			result, err := service.Unmanage(dryRun)
			return c.finishLifecycle(result, err, dryRun, asJSON)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the unmanage plan without changing anything")
	cmd.Flags().BoolVar(&asJSON, "json", false, "With --dry-run, print stable plan_schema=1 JSON")
	return cmd
}

func (c *commandLine) revertCommand() *cobra.Command {
	var asJSON, dryRun, force, list bool
	var to int
	cmd := &cobra.Command{
		Use: "revert", Short: "Restore the targets as they were before recent plans",
		Long: "Undo the latest journaled plan (or the latest --to <n> plans) by restoring each changed destination's previous link, " +
			"backup or absence and its ownership. Destinations changed since are conflicts unless --force moves them aside to a backup. " +
			"--list shows the journal, newest first. The next sync reapplies the manifest unless the repository changes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := jsonNeedsDryRun(asJSON, dryRun || list); err != nil {
				return err
			}
			service, err := c.service(cmd.Context())
			if err != nil {
				return classified(err)
			}
			if list {
				history, listErr := service.History()
				if listErr != nil {
					return classified(listErr)
				}
				return classified(c.printHistory(history, asJSON))
			}
			result, err := service.Revert(to, force, dryRun)
			if undo.OutOfRange(err) {
				return &commandError{err: err, code: apply.Usage}
			}
			return c.finishLifecycle(result, err, dryRun, asJSON)
		},
	}
	cmd.Flags().IntVar(&to, "to", 1, "Revert the latest n journaled plans (see --list)")
	cmd.Flags().BoolVar(&force, "force", false, "Move destinations changed since aside to a backup instead of skipping them")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the revert plan without changing anything")
	cmd.Flags().BoolVar(&list, "list", false, "List journaled plans, newest first")
	cmd.Flags().BoolVar(&asJSON, "json", false, "With --dry-run or --list, print stable JSON")
	cmd.MarkFlagsMutuallyExclusive("list", "dry-run")
	cmd.MarkFlagsMutuallyExclusive("list", "force")
	return cmd
}

func jsonNeedsDryRun(asJSON, readOnly bool) error {
	if asJSON && !readOnly {
		return &commandError{err: errors.New("--json requires --dry-run"), code: apply.Usage}
	}
	return nil
}

func (c *commandLine) finishLifecycle(result workflow.LifecycleResult, err error, dryRun, asJSON bool) error {
	if printErr := printWarnings(c.stderr, result.Warnings); printErr != nil {
		return classified(printErr)
	}
	if err != nil {
		c.code = max(result.Outcome.Result.Code(), apply.ErrorCode(err))
		if printErr := printApply(c.stdout, c.stderr, result.Outcome.Result); printErr != nil {
			return classified(printErr)
		}
		return classified(err)
	}
	if !dryRun {
		c.code = result.Outcome.Result.Code()
		return classified(printApply(c.stdout, c.stderr, result.Outcome.Result))
	}
	if asJSON {
		data, jsonErr := result.Outcome.Plan.JSON()
		if jsonErr != nil {
			return classified(jsonErr)
		}
		_, jsonErr = c.stdout.Write(data)
		return classified(jsonErr)
	}
	return classified(writeDiff(c.stdout, result.Outcome.Plan, c.colour()))
}

// historyList is the stable revert --list --json document (history_schema 1).
type historyList struct {
	Plans         []undo.Applied `json:"plans"`
	HistorySchema int            `json:"history_schema"`
}

func (c *commandLine) printHistory(history []undo.Applied, asJSON bool) error {
	if history == nil {
		history = []undo.Applied{}
	}
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(historyList{HistorySchema: 1, Plans: history})
	}
	if len(history) == 0 {
		_, err := io.WriteString(c.stdout, "No journaled plans.\n")
		return err
	}
	for _, applied := range history {
		kind := "applied"
		if applied.Adopted {
			kind = "adopted"
		}
		if _, err := fmt.Fprintf(c.stdout, "%d  %s  %s %d path(s)  %s\n", applied.Number, applied.AppliedAt.Format(time.RFC3339), kind, len(applied.Paths), applied.ID); err != nil {
			return err
		}
	}
	return nil
}
