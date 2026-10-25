package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/core/initialize"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (c *commandLine) initScaffold(cmd *cobra.Command, args []string, flags *initFlags) error {
	if len(args) != 1 || flags.repo != "" {
		return fmt.Errorf("use init --scaffold <directory>: %w", fs.ErrInvalid)
	}
	dir, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if err = c.scaffoldChoices(cmd, flags); err != nil {
		return err
	}
	template := initialize.Template{Agents: flags.agents, Instances: flags.instances}
	if _, err = template.Manifest(); err != nil {
		return err
	}
	confirmed, err := c.initConfirm(cmd.Context(), flags.yes, "Create kitbag.toml, README.md, AGENTS.md and a Git repository?")
	if err != nil {
		return err
	}
	if !confirmed {
		return c.initMessage("Cancelled.\n")
	}
	return c.writeScaffold(cmd, dir, template)
}

func (c *commandLine) writeScaffold(cmd *cobra.Command, dir string, template initialize.Template) error {
	envBackend, _ := c.host.LookupEnv(vcs.EnvVar)
	selection, err := vcs.Select(c.flags.gitBackend, envBackend, "")
	if err != nil {
		return err
	}
	backend, _, err := vcs.Open(cmd.Context(), selection, gitFactories(c.host, vcs.Identity{}))
	if err != nil {
		return err
	}
	existing, err := initialize.Scaffold(cmd.Context(), c.host, backend, dir, template)
	if err != nil {
		return err
	}
	if existing {
		return c.initMessage("Already scaffolded: template files exist in %s; nothing was replaced.\n", dir)
	}
	return c.initMessage("Created configuration repository: %s\nReview and commit the template, add a remote and push, then run kitbag init <git-url>.\n", dir)
}

func (c *commandLine) scaffoldChoices(cmd *cobra.Command, flags *initFlags) error {
	detected, err := initialize.Detect(c.host)
	if err != nil {
		return err
	}
	flags.detected = detectionDescription(detected)
	if err = c.printDetected(detected); err != nil {
		return err
	}
	if !cmd.Flags().Changed("agents") {
		for _, agent := range detected {
			flags.agents = append(flags.agents, agent.Kind)
		}
		if len(flags.agents) == 0 {
			flags.agents = []string{"claude"}
		}
	}
	if flags.yes {
		return nil
	}
	if !c.host.IsTerminal(0) {
		return fmt.Errorf("interactive scaffold needs a terminal; supply --yes: %w", fs.ErrInvalid)
	}
	flags.instanceText = strings.Join(flags.instances, ",")
	form := scaffoldForm(flags, cmd.Flags().Changed("agents"), cmd.Flags().Changed("instance"))
	if err = form.WithOutput(c.stdout).RunWithContext(cmd.Context()); err != nil {
		return err
	}
	flags.parseInstances()
	return nil
}

func (f *initFlags) parseInstances() {
	f.instances = nil
	if strings.TrimSpace(f.instanceText) == "" {
		return
	}
	for _, instance := range strings.Split(f.instanceText, ",") {
		f.instances = append(f.instances, strings.TrimSpace(instance))
	}
}
