package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

type globalFlags struct {
	config, host, gitBackend, mode string
	verbose, quiet                 bool
}
type commandLine struct {
	host           platform.Host
	stdout, stderr io.Writer
	selection      vcs.Selection
	flags          globalFlags
	config         config.Config
	code           apply.Code
}

func (c *commandLine) root() *cobra.Command {
	root := &cobra.Command{
		Use: "kitbag", Short: "Synchronize coding-agent configuration", SilenceUsage: true, SilenceErrors: true, DisableAutoGenTag: true,
		RunE:              func(*cobra.Command, []string) error { return fmt.Errorf("%s", usage) },
		PersistentPreRunE: c.prepare,
	}
	root.SetOut(c.stdout)
	root.SetErr(c.stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	flags := root.PersistentFlags()
	flags.StringVar(&c.flags.config, "config", "", "Machine configuration file (default: <config>/config.toml)")
	flags.StringVar(&c.flags.host, "host", "", "Host override (before KITBAG_HOST and config host)")
	flags.StringVar(&c.flags.gitBackend, "git-backend", "", "Git backend: auto, system or embedded")
	flags.StringVar(&c.flags.mode, "mode", "", "Materialization mode: link or copy (before KITBAG_MODE and config)")
	flags.BoolVar(&c.flags.verbose, "verbose", false, "Show backend selection and working copy diagnostics")
	flags.BoolVar(&c.flags.quiet, "quiet", false, "Suppress successful sync change output")
	root.MarkFlagsMutuallyExclusive("verbose", "quiet")
	root.AddCommand(c.syncCommand(), c.planCommand(), c.statusCommand(), c.doctorCommand(), c.adoptCommand(), c.unmanageCommand(), c.revertCommand(), c.versionCommand())
	return root
}

func (c *commandLine) prepare(cmd *cobra.Command, _ []string) error {
	if err := cmd.ValidateFlagGroups(); err != nil {
		return err
	}
	filename := c.flags.config
	if filename == "" {
		filename, _ = c.host.LookupEnv("KITBAG_CONFIG")
	}
	if filename != "" {
		absolute, err := filepath.Abs(filename)
		if err != nil {
			return classified(err)
		}
		filename = absolute
	}
	cfg, err := config.Load(c.host, filename)
	if err != nil {
		return classified(err)
	}
	c.config = cfg
	envBackend, _ := c.host.LookupEnv(vcs.EnvVar)
	c.selection, err = vcs.Select(c.flags.gitBackend, envBackend, cfg.Git.Backend)
	if err != nil {
		return &commandError{err: err, code: apply.Usage}
	}
	if cmd.Name() == "version" {
		return classified(checkGitBackend(cmd.Context(), c.selection, c.host))
	}
	return nil
}

func (c *commandLine) service(ctx context.Context) (workflow.Service, error) {
	repo, err := c.config.WorkingCopy(c.host)
	if err != nil {
		return workflow.Service{}, err
	}
	backend, resolution, err := vcs.Open(ctx, c.selection, gitFactories(c.host, c.config.Git.Identity))
	if err != nil {
		return workflow.Service{}, err
	}
	if c.flags.verbose {
		if _, err = fmt.Fprintln(c.stderr, resolution.String()+"; working copy "+repo); err != nil {
			return workflow.Service{}, err
		}
	}
	store, err := state.New(c.host, c.host, state.Options{})
	if err != nil {
		return workflow.Service{}, err
	}
	return workflow.Service{Host: c.host, Backend: backend, Store: store, Config: c.config, Repository: repo, Version: toolVersion(), HostFlag: c.flags.host, ModeFlag: c.flags.mode, Now: time.Now, Adapters: c.adapters}, nil
}

// A development build has no release version; it may only consume manifests
// requiring at most 0.0.0-dev. It must never bypass a minimum-version guard.
func toolVersion() string {
	if version == "dev" {
		return "0.0.0-dev"
	}
	return version
}

func (c *commandLine) versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the tool version", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		_, err := fmt.Fprintln(c.stdout, "kitbag", version)
		return classified(err)
	}}
}

func (c *commandLine) syncCommand() *cobra.Command {
	var noPull, force bool
	cmd := &cobra.Command{Use: "sync", Short: "Pull and apply configuration in one pass", Long: "Fetch and fast-forward a clean working copy, then read its manifest and apply one plan. Local changes skip the pull. An unchanged successful run is silent.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, err := c.service(cmd.Context())
		if err != nil {
			return classified(err)
		}
		result, err := service.Sync(cmd.Context(), noPull, force)
		c.code = max(result.Code(), apply.ErrorCode(err))
		if printErr := c.printSync(result); printErr != nil {
			return classified(printErr)
		}
		return classified(err)
	}}
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "Use the existing working tree without fetching")
	cmd.Flags().BoolVar(&force, "force", false, "Replace owned materialized drift when one-time backup policy permits")
	return cmd
}

func (c *commandLine) planCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "plan", Short: "Show the local synchronization diff without applying it", Long: "Build a dry-run plan from the existing working tree. No fetch or recovery is performed. The logical target diff is coloured only on a terminal with NO_COLOR unset. JSON excludes file contents.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, err := c.service(cmd.Context())
		if err != nil {
			return classified(err)
		}
		result, err := service.Inspect()
		if err != nil {
			return classified(err)
		}
		return classified(c.printPlan(result, asJSON))
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable plan_schema=1 JSON")
	return cmd
}

func (c *commandLine) statusCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "status", Short: "Show upstream relation, last sync and local drift", Long: "Report ahead/behind against the cached upstream, the last sync attempt for this clone, drift and warnings. This command never fetches; use sync to refresh remote knowledge.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, err := c.service(cmd.Context())
		if err != nil {
			return classified(err)
		}
		result, err := service.Status(cmd.Context())
		if err != nil {
			return classified(err)
		}
		return classified(c.printStatus(result, asJSON))
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print stable status_schema=1 JSON")
	return cmd
}
