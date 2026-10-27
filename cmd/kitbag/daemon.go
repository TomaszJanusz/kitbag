package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/daemon"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/daemonhost"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/watch"
)

const (
	daemonDirectoryPerm fs.FileMode = 0o700
	daemonProbeTimeout              = 15 * time.Second
	// The scheduler is serial, so two Ps serve it and the local API; a lower GC
	// target bounds heap growth within a cycle (idle budget in docs/daemon.md).
	daemonProcs     = 2
	daemonGCPercent = 50
	// daemonExecutableCheck polls file identity cheaply; content is hashed only
	// after a changed identity settles across two observations.
	daemonExecutableCheck = 5 * time.Second
)

func (c *commandLine) daemonCommand() *cobra.Command {
	return &cobra.Command{Use: "daemon", Short: "Poll configuration, detect drift and serve the local API", Long: "Run in the foreground with a per-user lock, bounded JSON logs, debounced filesystem watches and daily release metadata checks. Remote changes use the normal safe sync workflow; local edits only trigger inspection. Stop with SIGINT or SIGTERM. Restart after changing daemon settings. When its executable file is replaced, for example by a package upgrade, the daemon finishes the current operation and exits with status 75 so that its service manager starts the new version.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := platform.ShutdownContext(cmd.Context())
		defer stop()
		if c.process {
			c.tuneDaemonRuntime()
		}
		return classified(c.runDaemon(ctx))
	}}
}

// tuneDaemonRuntime trades negligible CPU for idle memory in the long-lived
// process only. Explicit GOMAXPROCS or GOGC settings win.
func (c *commandLine) tuneDaemonRuntime() {
	if _, set := c.host.LookupEnv("GOMAXPROCS"); !set {
		runtime.GOMAXPROCS(min(runtime.GOMAXPROCS(0), daemonProcs))
	}
	if _, set := c.host.LookupEnv("GOGC"); !set {
		debug.SetGCPercent(daemonGCPercent)
	}
}

func (c *commandLine) runDaemon(ctx context.Context) (err error) {
	dirs, err := c.host.Dirs()
	if err != nil {
		return err
	}
	if err = c.host.MkdirAll(dirs.State, daemonDirectoryPerm); err != nil {
		return err
	}
	lock, err := c.host.TryLock(filepath.Join(dirs.State, "daemon.lock"))
	if errors.Is(err, platform.ErrLocked) {
		return fmt.Errorf("daemon already running for this user: %w", err)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if restart, refreshErr := c.startupRefresh(ctx); refreshErr != nil || restart {
		return refreshErr
	}
	token, err := api.EnsureToken(c.host)
	if err != nil {
		return err
	}
	listener, err := daemonListener(ctx, c.host)
	if err != nil {
		return err
	}
	return c.serveDaemon(ctx, listener, token, dirs)
}

// The daemon lock is held before probing/removing a crash-stale endpoint. An
// unsafe entry or a live listener is never replaced, including a KIT-29 server.
func daemonListener(ctx context.Context, host platform.Host) (net.Listener, error) {
	path, err := api.SocketPath(host)
	if err != nil {
		return nil, err
	}
	info, err := host.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return host.ListenLocal(path)
	}
	if err != nil {
		return nil, err
	}
	if info.Mode().Type() != fs.ModeSocket {
		return nil, fs.ErrPermission
	}
	if err = removeStaleSocket(ctx, host, path); err != nil {
		return nil, err
	}

	return host.ListenLocal(path)
}

func removeStaleSocket(ctx context.Context, host platform.Host, path string) error {
	probe, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	conn, err := host.DialLocal(probe, path)
	if err == nil {
		return errors.Join(errors.New("local API already running"), conn.Close())
	}
	if !errors.Is(err, platform.ErrNoDaemon) {
		return err
	}
	return host.Remove(path)
}

func (c *commandLine) serveDaemon(ctx context.Context, listener net.Listener, token string, dirs platform.Dirs) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := api.NewEvents(toolVersion())
	handlers := c.apiHandlers(token, events)
	runtime := &daemonRuntime{host: c.host, events: events, service: func(ctx context.Context) (workflow.Service, error) { return handlers.Service(ctx, c.apiOptions()) }}
	handlers.Check = runtime.lastCheck
	runtime.watcher, err = watch.New()
	logger := daemonhost.Log{Host: c.host, Directory: dirs.LogDir, Now: time.Now}
	if runtime.watcher != nil {
		defer func() { err = errors.Join(err, runtime.watcher.Close()) }()
	}
	var logErr error
	log := func(action string, failure error) {
		if writeErr := logger.Write(action, failure); writeErr != nil {
			logErr = writeErr
			cancel()
		}
	}
	runtime.log = log
	log("started", nil)
	log("watch", err)
	releases := daemonhost.Releases{Host: c.host, StateDir: dirs.State, URL: c.config.Daemon.ReleaseURL, Version: toolVersion(), Now: time.Now}
	options, parseErr := c.daemonOptions()
	if parseErr != nil {
		return errors.Join(parseErr, listener.Close())
	}
	checked, err := c.checkRecorder(runtime, log)
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	deps := daemon.Dependencies{Now: time.Now, Checked: checked, Clock: daemonhost.Clock{}, Probe: runtime.probe, Sync: runtime.sync, Scan: runtime.scan, Update: releases.Check, Publish: func(kind string) {
		if kind == "sync.finished" {
			events.Publish(runtime.result)
		} else {
			events.Publish(api.Event{Kind: kind})
		}
	}, Log: log, Jitter: pollJitter, Trim: debug.FreeOSMemory}
	if runtime.watcher != nil {
		deps.Watch = runtime.watcher.Hints()
	}
	runErr, serveErr := c.runDaemonLoop(ctx, cancel, listener, handlers, options, deps, log)
	log("stopped", runErr)
	return errors.Join(runErr, serveErr, logErr)
}

func (c *commandLine) runDaemonLoop(ctx context.Context, cancel context.CancelFunc, listener net.Listener, handlers api.Handlers, options daemon.Options, deps daemon.Dependencies, log func(string, error)) (error, error) {
	served := make(chan error, 1)
	go func() { serveErr := api.Serve(ctx, listener, handlers); cancel(); served <- serveErr }()
	executable := c.watchExecutable(log)
	if executable != nil {
		options.ExecutableCheck, deps.Replaced = c.executableCheck, executable.Replaced
	}
	runErr := daemon.Run(ctx, options, deps)
	cancel()
	serveErr := <-served
	return c.restartError(runErr, executable), serveErr
}

func (c *commandLine) daemonOptions() (daemon.Options, error) {
	poll, err := time.ParseDuration(c.config.Sync.PollInterval)
	if err != nil {
		return daemon.Options{}, err
	}
	debounce, err := time.ParseDuration(c.config.Daemon.Debounce)
	if err != nil {
		return daemon.Options{}, err
	}
	enabled := c.config.Daemon.UpdateCheck != nil && *c.config.Daemon.UpdateCheck
	return daemon.Options{Poll: poll, Debounce: debounce, UpdateCheck: enabled}, nil
}

// restartError selects the restart exit status after a replaced executable.
func (c *commandLine) restartError(err error, executable *daemonhost.Executable) error {
	if !errors.Is(err, daemon.ErrReplaced) {
		return err
	}
	c.code = apply.Code(service.RestartExitCode)
	return fmt.Errorf("%w: %s; exiting with status %d so that the service manager starts the new version", err, executable.Path(), service.RestartExitCode)
}

// watchExecutable follows the executable path that service install writes, so
// a package manager's stable link that moves to a new version is a replacement.
// Without a usable path the daemon keeps running without the check.
func (c *commandLine) watchExecutable(log func(string, error)) *daemonhost.Executable {
	if c.executableCheck == 0 {
		c.executableCheck = daemonExecutableCheck
	}
	layout, err := c.host.ServiceLayout()
	if err != nil {
		log("executable", err)
		return nil
	}
	executable, err := daemonhost.WatchExecutable(c.host, layout.Binary, fileDigest)
	if err != nil {
		log("executable", err)
		return nil
	}
	return executable
}

// fileDigest streams a SHA-256 digest, so hashing the executable does not hold
// its whole content in the daemon's heap.
func fileDigest(path string) (sum []byte, err error) {
	file, err := os.Open(path) //nolint:gosec // G304: the path is this program's own executable.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

func pollJitter(delay time.Duration) time.Duration {
	const tenth = 10
	return delay - delay/tenth + time.Duration(rand.Int64N(int64(delay/tenth+1))) //nolint:gosec // G404: scheduling jitter does not require cryptographic randomness.
}

type daemonRuntime struct {
	check   *state.CheckRecord
	host    platform.Host
	service func(context.Context) (workflow.Service, error)
	watcher *watch.Watcher
	events  *api.Events
	log     func(string, error)
	result  api.Event
	checkMu sync.Mutex
}

func (r *daemonRuntime) probe(ctx context.Context) (string, error) {
	s, err := r.service(ctx)
	if err != nil {
		return "", err
	}
	probe, cancel := context.WithTimeout(ctx, daemonProbeTimeout)
	defer cancel()
	return s.Backend.RemoteHead(probe, s.Repository)
}

func (r *daemonRuntime) sync(ctx context.Context) (hash string, failure error) {
	r.result = api.Event{Kind: "sync.finished"}
	defer func() { r.result.ExitCode = max(r.result.ExitCode, int(apply.ErrorCode(failure))) }()
	s, err := r.service(ctx)
	if err != nil {
		return "", err
	}
	result, err := s.Sync(ctx, false, false)
	r.result.ExitCode = int(result.Code())
	r.result.Changed = len(result.Apply.Changed) + len(result.Includes.Added)
	if err != nil {
		return "", err
	}
	if result.Code() != apply.OK {
		return "", fmt.Errorf("sync needs attention (exit %d)", result.Code())
	}
	head, err := s.Backend.Head(ctx, s.Repository)
	if err != nil {
		return "", err
	}
	r.log("scan", r.scan(ctx))
	return head.Commit, nil
}

func (r *daemonRuntime) scan(ctx context.Context) error {
	s, err := r.service(ctx)
	if err != nil {
		return err
	}
	inspected, err := s.Inspect()
	if err != nil {
		return err
	}
	roots := make([]string, 0, len(inspected.Plan.Observations)+1)
	roots = append(roots, s.Repository)
	drift := false
	for _, observation := range inspected.Plan.Observations {
		roots = append(roots, observation.Path)
		drift = drift || observation.Classification != plan.InSync
	}
	status, err := s.Backend.Status(ctx, s.Repository)
	if err != nil {
		return err
	}
	if drift || !status.Clean() {
		r.events.Publish(api.Event{Kind: "drift.detected"})
	}
	if r.watcher != nil {
		return r.watcher.Rebuild(r.host, roots)
	}
	return nil
}

func (r *daemonRuntime) lastCheck(repository string) *state.CheckRecord {
	r.checkMu.Lock()
	defer r.checkMu.Unlock()
	if r.check == nil || r.check.Repository != repository {
		return nil
	}
	record := *r.check
	return &record
}

func (c *commandLine) checkRecorder(r *daemonRuntime, log func(string, error)) (func(state.CheckRecord), error) {
	store, err := state.New(c.host, c.host, state.Options{})
	if err != nil {
		return nil, err
	}
	repo, err := c.config.WorkingCopy(c.host)
	if err != nil {
		return nil, err
	}
	return func(record state.CheckRecord) {
		record.Repository = repo
		r.checkMu.Lock()
		r.check = &record
		r.checkMu.Unlock()
		log("poll.record", store.SaveCheck(record))
		r.events.Publish(api.Event{Kind: "poll.finished"})
	}, nil
}
