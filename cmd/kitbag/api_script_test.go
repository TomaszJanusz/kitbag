//go:build darwin || linux

package main

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/e2e"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

type (
	scriptAPIKey struct{}
	scriptAPI    struct {
		cancel  context.CancelFunc
		done    chan error
		runtime string
		once    sync.Once
	}
)

func configureAPIScripts(params *testscript.Params) {
	params.Setup = func(env *testscript.Env) error {
		if err := e2e.Setup(env); err != nil {
			return err
		}
		env.Values[scriptAPIKey{}] = &scriptAPI{}
		return nil
	}
	params.Cmds["localapi"] = localAPICommand
}

func localAPICommand(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: localapi start|stop")
	}
	server, ok := ts.Value(scriptAPIKey{}).(*scriptAPI)
	if !ok {
		ts.Fatalf("missing API fixture")
	}
	switch args[0] {
	case "start":
		server.start(ts, toolVersion())
	case "start-old":
		server.start(ts, "0.1.1")
	case "start-unversioned":
		server.start(ts, "")
	case "stop":
		server.stop(ts)
	default:
		ts.Fatalf("usage: localapi start|stop")
	}
}

func (s *scriptAPI) start(ts *testscript.TestScript, version string) {
	if s.cancel != nil {
		ts.Fatalf("API fixture already started")
	}
	runtime := s.runtime
	if runtime == "" {
		var err error
		runtime, err = os.MkdirTemp("/tmp", "kitbag-api-")
		ts.Check(err)
		s.runtime = runtime
		ts.Defer(func() { ts.Check(os.RemoveAll(runtime)) })
	}
	ts.Setenv("XDG_RUNTIME_DIR", runtime)
	// Snapshot only the script's environment. Server goroutines never access the
	// mutable testscript environment, real HOME or ambient agent directories.
	env := map[string]string{}
	for _, key := range []string{"HOME", "PATH", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "KITBAG_CONFIG", "KITBAG_GIT_BACKEND", "KITBAG_HOST", "KITBAG_MODE", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"} {
		env[key] = ts.Getenv(key)
	}
	host := unix.New(unix.WithEnv(func(key string) (string, bool) { value, ok := env[key]; return value, ok }))
	token, err := api.EnsureToken(host)
	ts.Check(err)
	path, err := api.SocketPath(host)
	ts.Check(err)
	listener, err := host.ListenLocal(path)
	ts.Check(err)
	ctx, cancel := context.WithCancel(context.Background())
	s.once = sync.Once{}
	s.cancel, s.done = cancel, make(chan error, 1)
	cli := &commandLine{host: host, stdout: io.Discard, stderr: io.Discard}
	handlers := cli.apiHandlers(token, api.NewEvents(version))
	handlers.ToolVersion = version
	go func() { s.done <- api.Serve(ctx, listener, handlers) }()
	ts.Defer(func() { s.stop(ts) })
}

func (s *scriptAPI) stop(ts *testscript.TestScript) {
	if s.cancel == nil {
		return
	}
	s.once.Do(func() {
		s.cancel()
		select {
		case err := <-s.done:
			ts.Check(err)
		case <-time.After(5 * time.Second):
			ts.Fatalf("API fixture did not stop")
		}
	})
	s.cancel = nil
}
