package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	uitext "github.com/TomaszJanusz/kitbag/internal/ui/model/text"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func printApply(stdout, stderr io.Writer, result apply.Result) error {
	for _, op := range result.Changed {
		if _, err := fmt.Fprintln(stdout, op.Kind, op.Path); err != nil {
			return err
		}
	}
	for _, d := range append(result.Warnings, result.Conflicts...) {
		if _, err := fmt.Fprintln(stderr, d.Path+":", d.Message); err != nil {
			return err
		}
	}
	return nil
}

func printIncludes(stdout, stderr io.Writer, result vcs.IncludeResult) error {
	for _, path := range result.Added {
		if _, err := fmt.Fprintln(stdout, "include", path); err != nil {
			return err
		}
	}
	return printWarnings(stderr, result.Warnings)
}

func printWarnings(out io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(out, warning); err != nil {
			return err
		}
	}
	return nil
}

func (c *commandLine) printSync(result workflow.SyncResult) error {
	stdout := c.stdout
	if c.flags.quiet {
		stdout = io.Discard
	}
	if err := printApply(stdout, c.stderr, result.Apply); err != nil {
		return err
	}
	if err := printIncludes(stdout, c.stderr, result.Includes); err != nil {
		return err
	}
	if result.Pull == vcs.PullSkippedDirty || result.Pull == vcs.PullDiverged {
		if _, err := fmt.Fprintln(c.stderr, "pull", result.Pull.String()); err != nil {
			return err
		}
	}
	return printWarnings(c.stderr, result.Warnings)
}

// colourDisabled honours NO_COLOR (any value) and ui.color=never.
func (c *commandLine) colourDisabled() bool {
	_, set := c.host.LookupEnv("NO_COLOR")
	return set || c.config.UI.Color == "never"
}

func (c *commandLine) colour() bool {
	if c.colourDisabled() {
		return false
	}
	if c.config.UI.Color == "always" {
		return true
	}
	fd, ok := c.stdout.(interface{ Fd() uintptr })
	return ok && c.host.IsTerminal(fd.Fd())
}

func (c *commandLine) printPlan(result workflow.Inspection, asJSON bool) error {
	if err := printWarnings(c.stderr, result.Warnings); err != nil {
		return err
	}
	if asJSON {
		data, err := result.Plan.JSON()
		if err != nil {
			return err
		}
		_, err = c.stdout.Write(data)
		return err
	}
	return writeDiff(c.stdout, result.Plan, c.colour())
}

func writeDiff(out io.Writer, p plan.Plan, colour bool) error {
	if len(p.Operations) == 0 {
		_, err := io.WriteString(out, "No changes.\n")
		return err
	}
	for _, op := range p.Operations {
		prefix, ansi := diffStyle(op.Kind)
		line := fmt.Sprintf("%s %s %q [%s]", prefix, op.Kind, op.Path, op.Classification)
		if op.Desired != nil && op.Desired.SourcePath != "" {
			line += " <- " + fmt.Sprintf("%q", op.Desired.SourcePath)
		}
		if op.Restore != nil && op.Restore.Backup+op.Restore.Link != "" {
			line += " <- " + fmt.Sprintf("%q", op.Restore.Backup+op.Restore.Link)
		}
		if op.Warning != "" {
			line += ": " + op.Warning
		}
		if colour {
			line = ansi + line + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func planOf(operations []plan.Operation) plan.Plan {
	return plan.Plan{PlanSchema: plan.SchemaVersion, Operations: operations, Observations: []plan.Observation{}}
}

func diffStyle(kind plan.Kind) (string, string) {
	switch kind {
	case plan.Remove, plan.Discard:
		return "-", "\x1b[31m"
	case plan.Backup, plan.Record, plan.Release, plan.Unmerge:
		return "~", "\x1b[36m"
	case plan.Materialize, plan.Write, plan.Merge, plan.Restore:
		return "+", "\x1b[32m"
	case plan.Skip:
		return "!", "\x1b[33m"
	}
	return "?", "\x1b[33m"
}

func (c *commandLine) printStatus(status workflow.Status, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	if status.ToolTooOld != nil {
		line := "tool too old: requires kitbag " + status.ToolTooOld.Required + "; running " + status.ToolTooOld.Current
		if status.ToolTooOld.Schema != 0 {
			line = fmt.Sprintf("tool too old: manifest schema %d needs a newer kitbag; running %s, which reads schema %d", status.ToolTooOld.Schema, status.ToolTooOld.Current, status.ToolTooOld.SupportedSchema)
		}
		if c.colour() {
			line = "\x1b[31m" + line + "\x1b[0m"
		}
		_, err := fmt.Fprintln(c.stdout, line)
		return err
	}
	last := "never"
	if status.LastSync != nil {
		last = fmt.Sprintf("%s; exit %d, %d changes", c.clock.Describe(status.LastSync.At), status.LastSync.Result, status.LastSync.Changed)
	}
	text := fmt.Sprintf("Working copy: %s\nBranch: %s -> %s (ahead %d, behind %d, dirty %t)\nLast sync: %s\n", status.Repository, status.Git.Branch, status.Git.Upstream, status.Git.Ahead, status.Git.Behind, status.Git.Dirty, last)
	if status.DaemonRunning {
		text += fmt.Sprintf("Versions: daemon %s, binary %s\n", status.DaemonVersion, status.BinaryVersion)
	}
	checkText, warnings := c.checkStatus(status)
	text += checkText
	var counts []string
	for _, kind := range []plan.Classification{plan.InSync, plan.UpdateNeeded, plan.LocalDrift, plan.Conflict, plan.UnmanagedObstacle} {
		counts = append(counts, fmt.Sprintf("%s: %d", kind, status.Drift[kind]))
	}
	if _, err := fmt.Fprintln(c.stdout, text+"Drift: "+strings.Join(counts, ", ")); err != nil {
		return err
	}
	return printWarnings(c.stderr, warnings)
}

func (c *commandLine) checkStatus(status workflow.Status) (string, []string) {
	var check *model.LastCheck
	if r := status.LastCheck; r != nil {
		check = &model.LastCheck{At: r.At, NextAt: r.NextAt, Outcome: r.Outcome, Category: r.Category, Interval: time.Duration(r.IntervalSeconds * float64(time.Second))}
	}
	catalog := uitext.English()
	checkText := "Last check: " + catalog.Render(model.CheckValue(c.clock, check, status.DaemonRunning)) + "\n"
	if status.DaemonRunning && check != nil {
		checkText += "Next check: " + c.clock.Stamp(check.NextAt) + "\n"
	}
	warnings := append([]string{}, status.Warnings...)
	if check != nil && check.Outcome == "failed" {
		warnings = append(warnings, "Last check: "+catalog.Render(model.CheckValue(c.clock, check, status.DaemonRunning)))
	}
	if status.DaemonRunning && c.clock.Now != nil && model.CheckStale(check, c.clock.Now()) {
		warnings = append(warnings, catalog.Render(uitext.New(uitext.StatusCheckStuck)))
	}
	return checkText, warnings
}
