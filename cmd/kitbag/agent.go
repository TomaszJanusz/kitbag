package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

const probeHelp = "Detection never lists HOME on its own. Only an explicit request (kitbag agent list --probe or Look for more directories in the terminal interface) reads the names of HOME's direct entries, once, without descending into or following any of them. Names that do not match a known agent directory are discarded without being shown or stored. Detection never reads agent settings, credentials, sessions or instance files, and never runs an agent executable."

func (c *commandLine) agentCommand() *cobra.Command {
	group := &cobra.Command{Use: "agent", Short: "Inspect and change agent kinds and instances"}
	var asJSON, probe bool
	list := &cobra.Command{Use: "list", Short: "List agent directories and declared instances", Long: "Show known native agent kinds, declared instances and undeclared directory candidates using the caller's relocation variables. Read-only: no Git, daemon, state lock or writes. An absent configuration or manifest is reported without preventing discovery.", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		result, err := c.listAgents(probe)
		if err != nil {
			var invalid *manifest.Error
			if errors.As(err, &invalid) && apply.ErrorCode(err) != apply.ToolTooOld {
				return &commandError{err: err, code: apply.Usage}
			}
			return classified(err)
		}
		return classified(c.printAgents(result, asJSON))
	}}
	list.Flags().BoolVar(&asJSON, "json", false, "Print stable agents_schema=1 JSON")
	list.Flags().BoolVar(&probe, "probe", false, probeHelp)
	group.AddCommand(list)
	group.AddCommand(c.agentEditCommands()...)
	return group
}

func (c *commandLine) listAgents(probe bool) (discover.Snapshot, error) {
	current, _, err := config.Resolve(c.host, config.Values{"host": c.config.Host}, "host", c.flags.host)
	if err != nil {
		return discover.Snapshot{}, err
	}
	m, warning, err := c.agentManifest()
	if err != nil {
		return discover.Snapshot{}, err
	}
	result, err := agentHomes().List(c.host, m, current, probe)
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, err
}

func (c *commandLine) agentManifest() (*manifest.Manifest, string, error) {
	if c.config.Repo.URL == "" {
		return nil, "No repository configured; listing detected agent directories only.", nil
	}
	repo, err := c.config.WorkingCopy(c.host)
	if err != nil {
		return nil, "", err
	}
	_, err = c.host.Lstat(filepath.Join(repo, "kitbag.toml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "No manifest yet; listing detected agent directories only.", nil
	}
	if err != nil {
		return nil, "", err
	}
	m, err := compatibility.Load(c.host, repo, toolVersion())
	return m, "", err
}

func (c *commandLine) printAgents(result discover.Snapshot, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	for _, kind := range result.Kinds {
		if _, err := fmt.Fprintln(c.stdout, kind+":"); err != nil {
			return err
		}
		if err := c.printAgentKind(kind, result); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintln(c.stderr, "kitbag: "+warning); err != nil {
			return err
		}
	}
	return nil
}

func (c *commandLine) printAgentKind(kind string, result discover.Snapshot) error {
	for _, instance := range result.Instances {
		if instance.Kind != kind {
			continue
		}
		detail := fmt.Sprintf("%s/%s hosts=%s excluded_hosts=%s settings=%t enabled=%t active_here=%t", instance.Target, instance.Name, hostScope(instance.Hosts), strings.Join(instance.ExcludedHosts, ","), instance.Settings, instance.Enabled, instance.ActiveHere)
		if err := c.printAgentDirectory(instance.ListedCandidate, detail); err != nil {
			return err
		}
	}
	for _, candidate := range result.Candidates {
		if candidate.Kind == kind {
			if err := c.printAgentDirectory(candidate, "undeclared"); err != nil {
				return err
			}
		}
	}
	return nil
}

func hostScope(hosts []string) string {
	if len(hosts) == 0 {
		return "all"
	}
	return strings.Join(hosts, ",")
}

func (c *commandLine) printAgentDirectory(candidate discover.ListedCandidate, detail string) error {
	source := string(candidate.Source)
	if candidate.Variable != "" {
		source += " (" + candidate.Variable + ")"
	}
	_, err := fmt.Fprintf(c.stdout, "  %s  %s  %s  %s\n", candidate.Dir, source, candidate.Presence, detail)
	return err
}
