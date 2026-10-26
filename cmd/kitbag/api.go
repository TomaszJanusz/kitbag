package main

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
)

// Verbose includes backend diagnostics, so it deliberately resolves and runs
// locally. Ordinary inspections use the daemon when available.
func (c *commandLine) inspect(ctx context.Context) (workflow.Inspection, error) {
	if _, err := c.minimumVersion(); err != nil {
		return workflow.Inspection{}, err
	}
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
	if repo, err := c.minimumVersion(); err != nil {
		return workflow.Status{StatusSchema: 1, Repository: repo, ToolTooOld: compatibility.FromError(err, repo), Warnings: []string{}, Drift: map[plan.Classification]int{plan.InSync: 0, plan.UpdateNeeded: 0, plan.LocalDrift: 0, plan.Conflict: 0, plan.UnmanagedObstacle: 0}}, err
	}
	local := func() (workflow.Status, error) {
		service, err := c.service(ctx)
		if err != nil {
			return workflow.Status{}, err
		}
		return service.Status(ctx)
	}
	if c.flags.verbose {
		snapshot, err := c.verboseStatus(ctx, local)
		snapshot.BinaryVersion = toolVersion()
		if err == nil {
			c.statusService(ctx, &snapshot)
		}
		return snapshot, err
	}
	options := c.apiOptions()
	result, err := (client.Client{Host: c.host}).Status(ctx, options, local)
	result.BinaryVersion = toolVersion()
	if err == nil {
		c.statusService(ctx, &result)
	}
	return result, err
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

// apiHandlers is the wiring boundary for the daemon and the real-socket
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

// minimumVersion guards API discovery too, even when an older daemon is running.
func (c *commandLine) minimumVersion() (string, error) {
	repo, err := c.config.WorkingCopy(c.host)
	if err != nil {
		return "", err
	}
	return repo, compatibility.Check(c.host, repo, toolVersion(), nil)
}

// verboseStatus preserves local backend diagnostics while obtaining live check
// metadata from the daemon; the core inspection still runs in this process.
func (c *commandLine) verboseStatus(ctx context.Context, local func() (workflow.Status, error)) (workflow.Status, error) {
	snapshot, err := local()
	if err != nil {
		return snapshot, err
	}
	live, err := (client.Client{Host: c.host}).Status(ctx, c.apiOptions(), func() (workflow.Status, error) { return snapshot, nil })
	snapshot.LastCheck, snapshot.DaemonRunning = live.LastCheck, live.DaemonRunning
	return snapshot, err
}
