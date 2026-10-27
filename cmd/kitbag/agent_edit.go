package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

// previewRounds bounds how often an interactive change is shown again
// because kitbag.toml moved upstream between preview and confirmation.
const previewRounds = 3

const agentEditLong = `Changes are written to the shared kitbag.toml in the working copy. An instance
applies to every host unless --hosts restricts it, which needs manifest schema 2
(kitbag manifest upgrade converts a schema-1 file). The working copy must
be clean, on its configured branch and without unpublished commits. KitBag
fast-forwards it, edits only the affected lines (comments and layout stay),
commits with a generated Conventional Commit message (Kitbag-Host and Kitbag-Op
trailers) and, when this host's publish mode is direct, pushes without force; a
rejected push is replayed once on the new upstream. A published change is then
synchronized. With publish mode pr nothing is written: the preview and the
commands to propose it are printed. A preview is shown first; without a terminal
pass --yes, or --dry-run to only preview.`

type agentEditFlags struct {
	settings                        string
	hosts                           []string
	asJSON, dryRun, yes, changedDir bool
	allHosts                        bool
}

func (c *commandLine) agentEditCommands() []*cobra.Command {
	var add agentEditFlags
	var change agents.Change
	addCmd := &cobra.Command{Use: "add", Short: "Declare an agent instance in kitbag.toml and publish it", Long: "Add an instance (a configuration directory) of a native agent kind. Without --target the kind's only target is used, or a target named after the kind is created. Adding the first listed instance also declares the kind's default directory, because listing instances replaces the implicit default. A directory that does not exist yet is allowed with a warning.\n\n" + agentEditLong, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		change.Op = agents.Add
		if change.Kind == "" || change.Name == "" || change.Dir == "" {
			return &commandError{err: errors.New("agent add needs --agent, --name and --dir"), code: apply.Usage}
		}
		return c.runAgentChange(cmd, change, add)
	}}
	addCmd.Flags().StringVar(&change.Kind, "agent", "", "Agent kind: claude, codex, gemini or opencode")
	addCmd.Flags().StringVar(&change.Name, "name", "", "Instance name (lowercase letters, digits, dots, underscores, hyphens)")
	addCmd.Flags().StringVar(&change.Dir, "dir", "", "Configuration directory below HOME (~/... or absolute)")
	addCmd.Flags().StringVar(&change.Target, "target", "", "Target to add to when several use the kind")
	c.agentEditFlags(addCmd, &add)
	c.agentHostsFlags(addCmd, &add, "Use the instance only on these hosts (comma-separated; manifest schema 2); default all hosts")
	return []*cobra.Command{addCmd, c.agentEditCommand(), c.agentRemoveCommand()}
}

func (c *commandLine) agentEditCommand() *cobra.Command {
	var flags agentEditFlags
	var dir string
	cmd := &cobra.Command{Use: "edit <target-or-kind>/<name>", Short: "Change an instance's directory, settings merge or hosts and publish it", Long: "Change the directory, the settings switch or the hosts of a declared instance. --hosts replaces its hosts list and --all-hosts removes it (manifest schema 2). The instance is named by its target, or by an agent kind used by exactly one target, and its name.\n\n" + agentEditLong, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		change, err := instanceRef(agents.Edit, args[0])
		if err != nil {
			return &commandError{err: err, code: apply.Usage}
		}
		change.Dir = dir
		flags.changedDir = cmd.Flags().Changed("dir")
		return c.runAgentChange(cmd, change, flags)
	}}
	cmd.Flags().StringVar(&dir, "dir", "", "New configuration directory below HOME (~/... or absolute)")
	c.agentEditFlags(cmd, &flags)
	c.agentHostsFlags(cmd, &flags, "Use the instance only on these hosts (comma-separated; manifest schema 2)")
	return cmd
}

func (c *commandLine) agentRemoveCommand() *cobra.Command {
	var flags agentEditFlags
	cmd := &cobra.Command{Use: "remove <target-or-kind>/<name>", Short: "Remove an instance from kitbag.toml and publish it", Long: "Remove an instance declaration. Neither the directory nor any file in it is deleted: the next sync removes only the links and unchanged copies KitBag made there and releases ownership; modified or unmanaged files stay and are reported. The last listed instance of a target cannot be removed, because the target would then manage its default directory.\n\n" + agentEditLong, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		change, err := instanceRef(agents.Remove, args[0])
		if err != nil {
			return &commandError{err: err, code: apply.Usage}
		}
		return c.runAgentChange(cmd, change, flags)
	}}
	c.agentCommonFlags(cmd, &flags)
	return cmd
}

func (c *commandLine) agentEditFlags(cmd *cobra.Command, flags *agentEditFlags) {
	cmd.Flags().StringVar(&flags.settings, "settings", "", "Merge the target's settings fragment into this instance: on or off")
	c.agentCommonFlags(cmd, flags)
}

func (c *commandLine) agentHostsFlags(cmd *cobra.Command, flags *agentEditFlags, usage string) {
	cmd.Flags().StringSliceVar(&flags.hosts, "hosts", nil, usage)
	cmd.Flags().BoolVar(&flags.allHosts, "all-hosts", false, "Use the instance on every host")
	cmd.MarkFlagsMutuallyExclusive("hosts", "all-hosts")
}

func (c *commandLine) agentCommonFlags(cmd *cobra.Command, flags *agentEditFlags) {
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Show the change, diff and commit message without writing")
	cmd.Flags().BoolVar(&flags.asJSON, "json", false, "Print stable agent_change_schema=1 JSON (needs --yes or --dry-run)")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Apply without asking for confirmation")
}

func instanceRef(op agents.Op, ref string) (agents.Change, error) {
	target, name, ok := strings.Cut(ref, "/")
	if !ok || target == "" || name == "" || strings.Contains(name, "/") {
		return agents.Change{}, fmt.Errorf("instance %q: use <target-or-kind>/<name>, for example claude/second", ref)
	}
	return agents.Change{Op: op, Target: target, Name: name}, nil
}

func (c *commandLine) runAgentChange(cmd *cobra.Command, change agents.Change, flags agentEditFlags) error {
	if err := c.agentChangeUsage(&change, flags); err != nil {
		return &commandError{err: err, code: apply.Usage}
	}
	service, err := c.service(cmd.Context())
	if err != nil {
		return classified(err)
	}
	if flags.yes || flags.dryRun {
		result, runErr := service.ChangeAgent(cmd.Context(), change, workflow.AgentOptions{DryRun: flags.dryRun})
		return c.finishAgentChange(result, runErr, flags.asJSON)
	}
	if !c.host.IsTerminal(0) {
		return &commandError{err: fmt.Errorf("confirming an agent change needs a terminal; pass --yes or --dry-run: %w", fs.ErrInvalid), code: apply.Usage}
	}
	return c.confirmAgentChange(cmd, service, change)
}

// agentChangeUsage rejects flag combinations outside the v1 slice.
func (c *commandLine) agentChangeUsage(change *agents.Change, flags agentEditFlags) error {
	if c.flags.host != "" {
		return errors.New("--host is not supported by agent add/edit/remove: kitbag.toml is shared by every host; restrict an instance with --hosts instead")
	}
	if err := hostsFlags(change, flags); err != nil {
		return err
	}
	if flags.asJSON && !flags.yes && !flags.dryRun {
		return errors.New("--json needs --yes or --dry-run")
	}
	if err := settingsFlag(change, flags.settings); err != nil {
		return err
	}
	if change.Op == agents.Edit && !requestsEdit(*change, flags) {
		return errors.New("nothing to change: pass --dir, --settings, --hosts or --all-hosts")
	}
	return nil
}

// requestsEdit reports whether edit was given anything to change.
func requestsEdit(change agents.Change, flags agentEditFlags) bool {
	return change.Settings != nil || flags.changedDir || change.Hosts != nil || change.AllHosts
}

// hostsFlags copies --hosts and --all-hosts; an empty list is a mistake.
func hostsFlags(change *agents.Change, flags agentEditFlags) error {
	if flags.hosts != nil && len(flags.hosts) == 0 {
		return errors.New("--hosts needs at least one host name; use --all-hosts for every host")
	}
	change.Hosts, change.AllHosts = flags.hosts, flags.allHosts
	return nil
}

func settingsFlag(change *agents.Change, value string) error {
	switch value {
	case "":
	case "on", "off":
		enabled := value == "on"
		change.Settings = &enabled
	default:
		return fmt.Errorf("--settings %q: use on or off", value)
	}
	return nil
}

func (c *commandLine) confirmAgentChange(cmd *cobra.Command, service workflow.Service, change agents.Change) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	for range previewRounds {
		preview, err := service.ChangeAgent(cmd.Context(), change, workflow.AgentOptions{DryRun: true})
		if err != nil {
			return c.finishAgentChange(preview, err, false)
		}
		if err = c.printAgentPreview(preview); err != nil {
			return classified(err)
		}
		ok, err := ask(c.stderr, scanner, "Apply this change? [y/N] ")
		if err != nil || !ok {
			return classified(err)
		}
		result, err := service.ChangeAgent(cmd.Context(), change, workflow.AgentOptions{Revision: preview.Plan.Revision})
		if !errors.Is(err, apply.ErrConflict) || result.Publication.State == workflow.Superseded {
			return c.finishAgentChange(result, err, false)
		}
		if _, err = fmt.Fprintln(c.stderr, "kitbag.toml changed upstream since this preview; here is the new one."); err != nil {
			return classified(err)
		}
	}
	return &commandError{err: fmt.Errorf("kitbag.toml keeps changing upstream; try again later: %w", apply.ErrConflict), code: apply.Conflict}
}

func ask(out io.Writer, scanner *bufio.Scanner, question string) (bool, error) {
	if _, err := fmt.Fprint(out, question); err != nil {
		return false, err
	}
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

func (c *commandLine) finishAgentChange(result workflow.AgentResult, err error, asJSON bool) error {
	c.code = max(c.code, result.Code())
	var printErr error
	switch {
	case asJSON && (err == nil || result.Publication.State != ""):
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		printErr = enc.Encode(result)
	case result.DryRun && err == nil:
		printErr = c.printAgentPreview(result)
	case result.Publication.State != "":
		printErr = c.printAgentResult(result)
	}
	if printErr != nil {
		return classified(printErr)
	}
	return classified(err)
}

func (c *commandLine) printAgentPreview(r workflow.AgentResult) error {
	// The change, manifest, commit and route lines frame the diff.
	const frame = 4
	lines := make([]string, 0, len(r.Plan.Diff)+frame)
	lines = append(lines, "Change: "+strings.TrimPrefix(r.Plan.Title, "chore(kitbag): "), "Manifest: "+r.Manifest)
	lines = append(lines, r.Plan.Diff...)
	route := "publish: direct push to " + r.Publication.Branch
	if r.Publication.Mode != "direct" {
		route = "publish: mode " + r.Publication.Mode + " (nothing is written; you get the commands to propose it)"
	}
	lines = append(lines, "Commit: "+r.Plan.Title, route)
	if _, err := fmt.Fprintln(c.stdout, strings.Join(lines, "\n")); err != nil {
		return err
	}
	return printWarnings(c.stderr, r.Plan.Warnings)
}

func (c *commandLine) printAgentResult(r workflow.AgentResult) error {
	if r.Publication.State == workflow.Manual {
		if err := c.printAgentPreview(r); err != nil {
			return err
		}
	}
	lines := append([]string{publicationLine(r)}, r.Publication.Next...)
	for i := 1; i < len(lines); i++ {
		lines[i] = "  " + lines[i]
	}
	if _, err := fmt.Fprintln(c.stdout, strings.Join(lines, "\n")); err != nil {
		return err
	}
	if err := c.printReleased(r); err != nil {
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
	return printWarnings(c.stderr, r.Plan.Warnings)
}

func publicationLine(r workflow.AgentResult) string {
	p := r.Publication
	switch p.State {
	case workflow.Published:
		return fmt.Sprintf("published %s to %s as %s", strings.TrimPrefix(r.Plan.Title, "chore(kitbag): "), p.Branch, shortCommit(p.Commit))
	case workflow.Manual:
		return "not written: " + p.Reason
	case workflow.NotPublished:
		return fmt.Sprintf("committed %s locally but not published: %s", shortCommit(p.Commit), p.Reason)
	case workflow.Superseded, workflow.PublishPreview:
	}
	return "not published: " + p.Reason
}

func (c *commandLine) printReleased(r workflow.AgentResult) error {
	if r.ReleaseResult == nil || r.Released == nil {
		return nil
	}
	if err := printApply(c.stdout, io.Discard, apply.Result{Changed: r.ReleaseResult.Changed}); err != nil {
		return err
	}
	return printWarnings(c.stderr, r.Released.Warnings)
}

func shortCommit(id string) string {
	const short = 12
	if len(id) > short {
		return id[:short]
	}
	return id
}
