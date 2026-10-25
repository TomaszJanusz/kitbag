package main

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

// Verbose includes backend diagnostics, so it deliberately resolves and runs
// locally. Ordinary inspections use the daemon when available.
func (c *commandLine) inspect(ctx context.Context) (workflow.Inspection, error) {
	local := func() (workflow.Inspection, error) {
		service, err := c.service(ctx)
		if err != nil {
			return workflow.Inspection{}, err
		}
		return service.Inspect()
	}
	if c.flags.verbose {
		return local()
	}
	options := c.apiOptions()
	return (client.Client{Host: c.host}).Inspect(ctx, options, local)
}

func (c *commandLine) status(ctx context.Context) (workflow.Status, error) {
	local := func() (workflow.Status, error) {
		service, err := c.service(ctx)
		if err != nil {
			return workflow.Status{}, err
		}
		return service.Status(ctx)
	}
	if c.flags.verbose {
		return local()
	}
	options := c.apiOptions()
	return (client.Client{Host: c.host}).Status(ctx, options, local)
}

func (c *commandLine) apiOptions() api.Options {
	host := c.flags.host
	if host == "" {
		host, _ = c.host.LookupEnv("KITBAG_HOST")
	}
	mode := c.flags.mode
	if mode == "" {
		mode, _ = c.host.LookupEnv("KITBAG_MODE")
	}
	return api.Options{Config: c.configPath, Host: host, Mode: mode, GitBackend: string(c.selection.Kind)}
}

// apiHandlers is the wiring boundary for a future daemon and the real-socket
// integration harness. Each request constructs independent command dependencies.
func (c *commandLine) apiHandlers(token string, events *api.Events) api.Handlers {
	return api.Handlers{Host: c.host, Token: token, Events: events, ToolVersion: toolVersion(), Service: func(ctx context.Context, options api.Options) (workflow.Service, error) {
		request := &commandLine{
			host: c.host, stdout: io.Discard, stderr: io.Discard,
			flags: globalFlags{config: options.Config, host: options.Host, mode: options.Mode, gitBackend: options.GitBackend},
		}
		cmd := &cobra.Command{Use: "status"}
		cmd.SetContext(ctx)
		if err := request.prepare(cmd, nil); err != nil {
			return workflow.Service{}, err
		}
		return request.service(ctx)
	}}
}
