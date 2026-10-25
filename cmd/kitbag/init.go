package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/initialize"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

type initFlags struct {
	selection                                vcs.Selection
	instanceText, detected                   string
	repo, branch, name, email, backend, mode string
	agents, instances                        []string
	yes, scaffold                            bool
}

func (c *commandLine) initCommand() *cobra.Command {
	var flags initFlags
	cmd := &cobra.Command{
		Use: "init [git-url]", Short: "Set up a configuration clone or scaffold a new repository",
		Long: "Clone a configuration repository, detect existing supported agent directories, show a dry plan and save machine configuration after confirmation. Existing configuration is never replaced. Use --yes for headless setup, or --scaffold <dir> for a new repository matching the minimal manifest example.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return classified(c.runInit(cmd, args, &flags)) },
	}
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Accept defaults and confirmations without reading terminal input")
	cmd.Flags().BoolVar(&flags.scaffold, "scaffold", false, "Create a new repository in the positional directory")
	cmd.Flags().StringVar(&flags.repo, "repo", "", "Repository URL (alternative to the positional URL)")
	cmd.Flags().StringVar(&flags.branch, "branch", "", "Repository branch (default: remote default)")
	cmd.Flags().StringVar(&flags.name, "identity-name", "", "Fallback unsigned embedded commit author name")
	cmd.Flags().StringVar(&flags.email, "identity-email", "", "Fallback unsigned embedded commit author email")
	cmd.Flags().StringSliceVar(&flags.agents, "agents", nil, "Scaffold agent kinds: claude,codex (default: detected, else claude)")
	cmd.Flags().StringArrayVar(&flags.instances, "instance", nil, "Scaffold instance: agent.name=~/directory (repeatable)")
	return cmd
}

func (c *commandLine) runInit(cmd *cobra.Command, args []string, flags *initFlags) error {
	if flags.scaffold {
		return c.initScaffold(cmd, args, flags)
	}
	if len(flags.agents) != 0 || len(flags.instances) != 0 {
		return fmt.Errorf("--agents and --instance require --scaffold: %w", fs.ErrInvalid)
	}
	filename, err := initialize.ConfigPath(c.host, c.flags.config)
	if err != nil {
		return err
	}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return err
	}
	exists, err := initialize.Exists(c.host, filename)
	if err != nil {
		return err
	}
	if exists {
		return c.initMessage("Already set up: %s exists; configuration and working copies were left unchanged.\n", filename)
	}
	if err = c.initInputs(args, flags); err != nil {
		return err
	}
	if err = c.initChoices(cmd, flags); err != nil {
		return err
	}
	return c.initClone(cmd.Context(), filename, flags)
}

func initRepo(args []string, flags *initFlags) error {
	if len(args) == 1 {
		if flags.repo != "" {
			return fmt.Errorf("provide the URL either positionally or with --repo: %w", fs.ErrInvalid)
		}
		flags.repo = args[0]
	}
	if flags.yes && flags.repo == "" {
		return fmt.Errorf("--yes requires a repository URL: %w", fs.ErrInvalid)
	}
	return nil
}

func (c *commandLine) initChoices(cmd *cobra.Command, flags *initFlags) error {
	envBackend, _ := c.host.LookupEnv(vcs.EnvVar)
	sel, err := vcs.Select(c.flags.gitBackend, envBackend, "")
	if err != nil {
		return err
	}
	flags.selection, flags.backend = sel, string(sel.Kind)
	flags.mode, _, err = config.Resolve(c.host, config.Values{}, "sync.mode", c.flags.mode)
	if err != nil {
		return err
	}
	detected, err := initialize.Detect(c.host)
	if err != nil {
		return err
	}
	flags.detected = detectionDescription(detected)
	if err = c.printDetected(detected); err != nil {
		return err
	}
	if flags.yes {
		return nil
	}
	if !c.host.IsTerminal(0) {
		return fmt.Errorf("interactive init needs a terminal; supply --yes and flags: %w", fs.ErrInvalid)
	}
	return initForm(flags, cmd.Flags().Changed("git-backend"), cmd.Flags().Changed("mode")).WithOutput(c.stdout).RunWithContext(cmd.Context())
}

func (c *commandLine) initClone(ctx context.Context, filename string, flags *initFlags) error {
	cfg := flags.configuration(c.flags.host)
	selection := flags.selection
	if string(selection.Kind) != flags.backend {
		selection = vcs.Selection{Kind: vcs.Kind(flags.backend), Source: vcs.SourceFlag}
	}
	backend, resolution, err := vcs.Open(ctx, selection, gitFactories(c.host, cfg.Git.Identity))
	if err != nil {
		return err
	}
	if err = c.initMessage("Git: %s\n", resolution.String()); err != nil {
		return err
	}
	service := initialize.Service{Host: c.host, Backend: backend, Config: cfg, Filename: filename, Version: toolVersion(), Adapters: c.adapters}
	repo, reused, err := service.Clone(ctx)
	if err != nil {
		return err
	}
	if err = c.initMessage("Working copy: %s (already exists: %t)\n", repo, reused); err != nil {
		return err
	}
	if err = c.initIdentity(ctx, repo, flags, service); err != nil {
		return err
	}
	service.Config = flags.configuration(c.flags.host)
	return c.initSave(ctx, service, repo, flags.yes)
}

func (c *commandLine) initSave(ctx context.Context, service initialize.Service, repo string, yes bool) error {
	inspection, err := service.Inspect(repo)
	if err != nil {
		return err
	}
	if err = c.printPlan(inspection, false); err != nil {
		return err
	}
	confirmed, err := c.initConfirm(ctx, yes, "Write machine configuration? Agent files change only when you run sync.")
	if err != nil {
		return err
	}
	if !confirmed {
		return c.initMessage("Cancelled; clone retained for a later init. No machine configuration was written.\n")
	}
	return c.saveInitial(service)
}

func (c *commandLine) saveInitial(service initialize.Service) error {
	if err := service.Save(); err != nil {
		exists, checkErr := initialize.Exists(c.host, service.Filename)
		if errors.Is(err, fs.ErrExist) && exists {
			return c.initMessage("Already set up: %s exists; it was not replaced.\n", service.Filename)
		}
		return errors.Join(err, checkErr)
	}
	return c.initMessage("Wrote %s\nRun manually: kitbag --config %s sync\nBackground sync: kitbag --config %s service install\n", service.Filename, shellQuote(service.Filename), shellQuote(service.Filename))
}

func (f initFlags) configuration(host string) config.Config {
	cfg := config.Config{Schema: 1, Host: host}
	cfg.Repo.URL, cfg.Repo.Branch = f.repo, f.branch
	cfg.Git.Backend, cfg.Git.Identity = f.backend, vcs.Identity{Name: f.name, Email: f.email}
	cfg.Sync.Mode = f.mode
	return cfg
}

func (c *commandLine) initIdentity(ctx context.Context, repo string, flags *initFlags, service initialize.Service) error {
	needed, err := service.NeedsIdentity(ctx, repo)
	if err != nil {
		return err
	}
	if !needed || flags.name != "" || flags.email != "" {
		return nil
	}
	if flags.yes {
		return c.initMessage("Commit identity is unset; sync works without it. Set git.identity before publishing with embedded Git.\n")
	}
	return identityForm(flags).WithOutput(c.stdout).RunWithContext(ctx)
}

func (c *commandLine) printDetected(agents []initialize.Agent) error {
	if len(agents) == 0 {
		return c.initMessage("Detected agent directories: none. Targets and instances are controlled by kitbag.toml.\n")
	}
	for _, agent := range agents {
		if err := c.initMessage("Detected %s: %s\n", agent.Kind, agent.Directory); err != nil {
			return err
		}
	}
	return c.initMessage("Targets and instances are controlled by kitbag.toml; detection does not change them.\n")
}

func (c *commandLine) initMessage(format string, values ...any) error {
	_, err := fmt.Fprintf(c.stdout, format, values...)
	return err
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (c *commandLine) initSettings(flags *initFlags, positional bool) error {
	for _, field := range []struct {
		value *string
		path  string
	}{
		{&flags.repo, "repo.url"},
		{&flags.branch, "repo.branch"},
		{&flags.name, "git.identity.name"},
		{&flags.email, "git.identity.email"},
		{&c.flags.host, "host"},
	} {
		if positional && field.path == "repo.url" {
			continue
		}
		value, _, err := config.Resolve(c.host, config.Values{}, field.path, *field.value)
		if err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}

func (c *commandLine) initInputs(args []string, flags *initFlags) error {
	if err := c.initSettings(flags, len(args) > 0); err != nil {
		return err
	}
	return initRepo(args, flags)
}
