package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/ui/apiclient"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/ui/model/text"
	"github.com/TomaszJanusz/kitbag/internal/ui/tui"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func (c *commandLine) tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use: "tui", Short: "Open the interactive terminal interface",
		Long: "Show status, drift, settings, agents and activity in a full-screen terminal interface; the Agents tab adds, edits and removes agent instances with a previewed, confirmed wizard. It uses the running daemon and falls back to reading this machine directly when none listens. Requires an interactive terminal; NO_COLOR or ui.color=never disables colour. Press ? for keys.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return classified(c.runTUI(cmd.Context(), cmd.InOrStdin()))
		},
	}
}

func (c *commandLine) runTUI(ctx context.Context, input io.Reader) error {
	if !c.terminal(input) || !c.terminal(c.stdout) {
		return fmt.Errorf("kitbag tui needs an interactive terminal on stdin and stdout: %w", fs.ErrInvalid)
	}
	return tui.Run(ctx, c.tuiOptions(tui.TerminalProfile(c.stdout)), input, c.stdout)
}

func (c *commandLine) terminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && c.host.IsTerminal(file.Fd())
}

// tuiOptions wires the shared models to the local API with the same
// in-process fallback as status, plan and config. Backend diagnostics
// (--verbose) are not written: stderr output would corrupt the screen.
func (c *commandLine) tuiOptions(profile tui.Profile) tui.Options {
	local := *c
	local.flags.verbose, local.stderr = false, io.Discard
	adapter := apiclient.Client{
		BinaryVersion: toolVersion(), Host: c.host, OpenGit: c.settingsGit,
		Transport: client.Client{Host: c.host}, Env: c.host, Options: c.apiOptions(),
		Flags:          map[string]string{"host": c.flags.host, "mode": c.flags.mode, "git-backend": c.flags.gitBackend},
		Local:          apiclient.Local{Workflow: local.service, Settings: local.settingsService},
		AgentVariables: agentVariables(),
	}
	return tui.Options{
		Version: toolVersion(), Runner: model.Runner{Client: adapter, Now: time.Now}, Catalog: text.English(),
		Profile: profile, NoColour: c.colourDisabled(), LogCapacity: model.DefaultLogCapacity, Clock: c.clock,
	}
}

// settingsGit opens the effective backend without KitBag's identity fallback:
// the settings hint describes Git config, not the value being edited.
func (c *commandLine) settingsGit(ctx context.Context, kind string) (vcs.Backend, error) {
	selection, err := vcs.Select("", "", kind)
	if err != nil {
		return nil, err
	}
	backend, _, err := vcs.Open(ctx, selection, gitFactories(c.host, vcs.Identity{}))
	return backend, err
}
