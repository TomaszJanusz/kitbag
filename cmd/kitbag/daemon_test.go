//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/core/service"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

type daemonFixture struct {
	host               platform.Host
	lookup             func(string) (string, bool)
	home, remote, seed string
	env                []string
}

func newDaemonFixture(t *testing.T) daemonFixture {
	t.Helper()
	root := daemonTestRoot(t)
	home := filepath.Join(root, "home")
	path, _ := newHost().LookupEnv("PATH")
	values := map[string]string{"HOME": home, "PATH": path, "XDG_RUNTIME_DIR": filepath.Join(root, "run"), "GIT_CONFIG_GLOBAL": filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM": "1"}
	lookup := func(key string) (string, bool) { value, ok := values[key]; return value, ok }
	host := unix.New(unix.WithEnv(lookup))
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	f := daemonFixture{host: host, lookup: lookup, home: home, remote: filepath.Join(root, "remote.git"), seed: filepath.Join(root, "seed"), env: env}
	t.Cleanup(func() { f.logFailure(t) })
	f.git(t, "init", "--quiet", "--bare", "--initial-branch=main", f.remote)
	f.git(t, "clone", "--quiet", f.remote, f.seed)
	f.write(t, filepath.Join(f.seed, "kitbag.toml"), "schema=1\n[[files]]\nsrc='source'\ndest='~/output'\nmode='copy'\n")
	f.write(t, filepath.Join(f.seed, "source"), "first")
	f.push(t, "seed")
	config := fmt.Sprintf("schema=1\n[repo]\nurl=%q\npath='~/repo'\n[git]\nbackend='embedded'\n[sync]\npoll_interval='1s'\n[daemon]\ndebounce='100ms'\nupdate_check=false\n", f.remote)
	f.write(t, filepath.Join(home, ".config", "kitbag", "config.toml"), config)
	return f
}

func daemonTestRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "kitbag-daemon-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			t.Error(removeErr)
		}
	})
	// Keep socket paths short, then resolve macOS's /tmp -> /private/tmp so
	// planner checks see real parents for both sources and destinations.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func (f daemonFixture) logFailure(t *testing.T) {
	t.Helper()
	if !t.Failed() {
		return
	}
	dirs, err := f.host.Dirs()
	if err != nil {
		t.Log(err)
		return
	}
	for _, path := range []string{filepath.Join(dirs.LogDir, "daemon.jsonl"), filepath.Join(dirs.State, "last-sync.json")} {
		data, readErr := f.host.ReadFile(path)
		t.Logf("%s: %s (%v)", path, data, readErr)
	}
}

func (f daemonFixture) write(t *testing.T, path, body string) {
	t.Helper()
	if err := f.host.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.host.WriteAtomic(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f daemonFixture) git(t *testing.T, args ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: fixed Git executable and fixture arguments, without a shell.
	command.Env = f.env
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func (f daemonFixture) push(t *testing.T, message string) {
	t.Helper()
	f.git(t, "-C", f.seed, "add", ".")
	f.git(t, "-C", f.seed, "-c", "user.name=Daemon Test", "-c", "user.email=daemon@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", message)
	f.git(t, "-C", f.seed, "push", "--quiet", "origin", "HEAD:main")
}

func daemonProcess(t *testing.T, f daemonFixture) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, executable, "-test.run=^TestDaemonProcessHelper$", "--") //nolint:gosec // G204: re-exec the test binary with fixed arguments.
	command.Env = f.env
	return command
}

// TestDaemonProcessHelper runs production command wiring with an isolated HOME.
func TestDaemonProcessHelper(t *testing.T) {
	t.Parallel()
	if slices.Index(os.Args, "--") < 0 {
		return
	}
	if code := run(runtime.GOOS, newHost(), []string{"daemon"}, os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("daemon exited %d", code)
	}
}

func TestDaemonRemoteLockDriftAndSIGTERM(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	command, output := startDaemonProcess(t, f)
	waitDaemonFile(t, f, "first")
	contender := daemonProcess(t, f)
	result, err := contender.CombinedOutput()
	if err == nil || !strings.Contains(string(result), "daemon already running") {
		t.Fatalf("second instance: %v %s", err, result)
	}
	subscriber := subscribeDaemon(t, f)
	f.write(t, filepath.Join(f.home, "output"), "manual drift")
	waitDaemonEvent(t, subscriber, "drift.detected")
	// Target drift remains unchanged across a poll; a watcher must never repair it.
	time.Sleep(1200 * time.Millisecond)
	data, err := f.host.ReadFile(filepath.Join(f.home, "output"))
	if err != nil || string(data) != "manual drift" {
		t.Fatalf("watcher rewrote drift: %q %v", data, err)
	}
	f.write(t, filepath.Join(f.home, "output"), "first")
	f.write(t, filepath.Join(f.seed, "source"), "remote update")
	started := time.Now()
	f.push(t, "update")
	waitDaemonFile(t, f, "remote update")
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("remote apply exceeded poll + jitter/test scheduling allowance: %s", elapsed)
	}
	working := subscribeDaemon(t, f)
	f.write(t, filepath.Join(f.home, "repo", "source"), "manual working-copy edit")
	waitDaemonEvent(t, working, "drift.detected")
	stopDaemonProcess(t, f, command, output)
	assertDaemonReleased(t, f)
}

func startDaemonProcess(t *testing.T, f daemonFixture) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	command := daemonProcess(t, f)
	output := new(bytes.Buffer)
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			if err := command.Wait(); err != nil {
				t.Log(err)
			}
		}
		if t.Failed() {
			t.Logf("daemon stdout/stderr: %s", output.String())
		}
	})
	return command, output
}

func stopDaemonProcess(t *testing.T, f daemonFixture, command *exec.Cmd, output *bytes.Buffer) {
	t.Helper()
	stop := exec.CommandContext(t.Context(), "kill", "-TERM", strconv.Itoa(command.Process.Pid)) //nolint:gosec // G204: kill receives only the PID of our own fixture process.
	stop.Env = f.env
	if err := stop.Run(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("SIGTERM: %v %s", err, output.String())
	}
}

func waitDaemonFile(t *testing.T, f daemonFixture, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := f.host.ReadFile(filepath.Join(f.home, "output"))
		if err == nil && string(data) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("daemon did not apply %q", want)
}

func subscribeDaemon(t *testing.T, f daemonFixture) *json.Decoder {
	t.Helper()
	path, err := api.SocketPath(f.host)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := f.host.DialLocal(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	token, err := api.LoadToken(f.host)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Write(append(apiRequest(t, "events.subscribe", "{}", token), '\n')); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bufio.NewReader(conn))
	var response api.Response
	if err = decoder.Decode(&response); err != nil || response.Error != nil {
		t.Fatalf("subscribe: %+v %v", response, err)
	}
	return decoder
}

func waitDaemonEvent(t *testing.T, decoder *json.Decoder, kind string) {
	t.Helper()
	for {
		var notification api.Notification
		if err := decoder.Decode(&notification); err != nil {
			t.Fatal(err)
		}
		if notification.Method == kind {
			return
		}
	}
}

func assertDaemonReleased(t *testing.T, f daemonFixture) {
	t.Helper()
	path, err := api.SocketPath(f.host)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.host.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket left: %v", err)
	}
	dirs, err := f.host.Dirs()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := f.host.TryLock(filepath.Join(dirs.State, "daemon.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestDaemonStaleSocketAndUnsafeEntries(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	path, err := api.SocketPath(f.host)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.host.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err = stale.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.host.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := daemonListener(t.Context(), f.host)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = daemonListener(t.Context(), f.host); err == nil {
		t.Fatal("replaced live socket")
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	f.write(t, path, "do not remove")
	if _, err = daemonListener(context.Background(), f.host); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("unsafe socket entry: %v", err)
	}
}

func TestDaemonContextCancellation(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	cli := &commandLine{host: f.host, stdout: io.Discard, stderr: io.Discard}
	root := cli.root()
	root.SetArgs([]string{"daemon"})
	ctx, cancel := context.WithCancel(t.Context())
	root.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("shutdown hung")
		}
		assertDaemonReleased(t, f)
	})
	waitDaemonFile(t, f, "first")
	// Cancel only after the initial sync, its watch scan and one remote poll
	// succeeded, so the in-process run deterministically covers each cycle.
	for _, action := range []string{"sync", "scan", "poll"} {
		waitDaemonLog(t, f, action, false, 1)
	}
}

func TestDaemonBrokenRemoteBackoff(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	command, output := startDaemonProcess(t, f)
	waitDaemonFile(t, f, "first")
	if err := f.host.Rename(f.remote, f.remote+".offline"); err != nil {
		t.Fatal(err)
	}
	failures := waitDaemonLog(t, f, "poll", true, 2)
	if gap := failures[1].Sub(failures[0]); gap < 1800*time.Millisecond {
		t.Fatalf("broken remote did not back off: %v", gap)
	}
	stopDaemonProcess(t, f, command, output)
	store, err := state.New(f.host, f.host, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := store.LoadCheck()
	if err != nil || record == nil {
		t.Fatal(record, err)
	}
	if record.Outcome != "failed" || record.Category != "operation" || record.Head != "" {
		t.Fatalf("failed check=%+v", record)
	}
}

// waitDaemonLog returns the times of at least count daemon log records with
// the given action and failure state.
func waitDaemonLog(t *testing.T, f daemonFixture, action string, failed bool, count int) []time.Time {
	t.Helper()
	dirs, err := f.host.Dirs()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := f.host.ReadFile(filepath.Join(dirs.LogDir, "daemon.jsonl"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		times := daemonLogTimes(t, data, action, failed)
		if len(times) >= count {
			return times
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("daemon did not log %d %s records with failed=%t", count, action, failed)
	return nil
}

func daemonLogTimes(t *testing.T, data []byte, action string, failed bool) []time.Time {
	t.Helper()
	times := []time.Time{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var record struct {
			At     time.Time
			Action string
			Failed bool
		}
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record.Action == action && record.Failed == failed {
			times = append(times, record.At)
		}
	}
	return times
}

// TestDaemonIdleHeap re-executes itself so the measured heap and runtime
// settings belong to one daemon only. Live heap must stay small and must not
// grow across applied remote changes (docs/daemon.md); RSS is measured manually.
func TestDaemonIdleHeap(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("starts a daemon and applies several remote changes")
	}
	if slices.Index(os.Args, "--") < 0 {
		runHeapChild(t)
		return
	}
	assertExplicitRuntimeSettingsWin(t)
	const (
		heapBound   = 8 << 20
		growthBound = 1 << 20
	)
	f := newDaemonFixture(t)
	writeHeapTree(t, f)
	defer startTunedDaemon(t, f)()
	waitDaemonFile(t, f, "first")
	if procs, percent := runtime.GOMAXPROCS(0), debug.SetGCPercent(daemonGCPercent); procs > daemonProcs || percent != daemonGCPercent {
		t.Fatalf("daemon runtime not tuned: GOMAXPROCS=%d GOGC=%d", procs, percent)
	}
	waitDaemonTrim(t)
	baseline := liveHeap()
	for i := range 3 {
		want := fmt.Sprintf("remote update %d", i)
		f.write(t, filepath.Join(f.seed, "source"), want)
		f.push(t, want)
		waitDaemonFile(t, f, want)
	}
	after := liveHeap()
	t.Logf("live heap: baseline alloc=%d inuse=%d; after alloc=%d inuse=%d", baseline.HeapAlloc, baseline.HeapInuse, after.HeapAlloc, after.HeapInuse)
	if after.HeapInuse > heapBound || after.HeapAlloc > baseline.HeapAlloc+growthBound {
		t.Fatalf("daemon heap retained across cycles: baseline=%d after=%d inuse=%d", baseline.HeapAlloc, after.HeapAlloc, after.HeapInuse)
	}
}

func runHeapChild(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, executable, "-test.run=^TestDaemonIdleHeap$", "-test.v", "--").CombinedOutput() //nolint:gosec // G204: re-exec the test binary with fixed arguments.
	if err != nil {
		t.Fatalf("heap child: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

// startTunedDaemon runs production wiring in this process, including the
// daemon-only runtime tuning, and returns a function that stops it.
func startTunedDaemon(t *testing.T, f daemonFixture) func() {
	t.Helper()
	cli := &commandLine{host: f.host, stdout: io.Discard, stderr: io.Discard, process: true}
	root := cli.root()
	root.SetArgs([]string{"daemon"})
	ctx, cancel := context.WithCancel(t.Context())
	root.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

// writeHeapTree adds 1 MiB of managed files, so retaining file data or trees
// between cycles would exceed the growth bound.
func writeHeapTree(t *testing.T, f daemonFixture) {
	t.Helper()
	const files, size = 32, 32 << 10
	for i := range files {
		f.write(t, filepath.Join(f.seed, "tree", fmt.Sprintf("file-%02d", i)), strings.Repeat(strconv.Itoa(i%10), size))
	}
	f.write(t, filepath.Join(f.seed, "kitbag.toml"), "schema=1\n[[files]]\nsrc='source'\ndest='~/output'\nmode='copy'\n[[files]]\nsrc='tree'\ndest='~/tree'\nmode='copy'\n")
	f.push(t, "tree")
}

func assertExplicitRuntimeSettingsWin(t *testing.T) {
	t.Helper()
	const procs, percent = 4, 100
	runtime.GOMAXPROCS(procs)
	debug.SetGCPercent(percent)
	explicit := &commandLine{host: unix.New(unix.WithEnv(func(key string) (string, bool) { return "1", key == "GOMAXPROCS" || key == "GOGC" }))}
	explicit.tuneDaemonRuntime()
	if got, gc := runtime.GOMAXPROCS(0), debug.SetGCPercent(percent); got != procs || gc != percent {
		t.Fatalf("explicit runtime settings overridden: GOMAXPROCS=%d GOGC=%d", got, gc)
	}
}

// waitDaemonTrim observes the scheduler's idle trim (debug.FreeOSMemory) as a
// forced collection; this test forces none before its first heap sample.
func waitDaemonTrim(t *testing.T) {
	t.Helper()
	forced := []metrics.Sample{{Name: "/gc/cycles/forced:gc-cycles"}}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if metrics.Read(forced); forced[0].Value.Uint64() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not trim idle memory after the initial sync")
}

// liveHeap keeps the smallest of several samples: retained memory shows in
// every sample, while a poll or scan in flight during one sample does not.
func liveHeap() runtime.MemStats {
	var least runtime.MemStats
	for i := range 10 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if i == 0 || stats.HeapAlloc < least.HeapAlloc {
			least = stats
		}
		time.Sleep(100 * time.Millisecond)
	}
	return least
}

// TestDaemonRestartsAfterExecutableReplaced replaces the watched executable
// on a temporary path: an identical reinstall keeps the daemon running, a
// different binary makes it exit with the restart status after releasing its
// locks, and the next daemon neither exits again nor restarts for a missing file.
func TestDaemonRestartsAfterExecutableReplaced(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	binary := filepath.Join(f.home, "bin", "kitbag")
	f.writeExecutable(t, binary, "#!/bin/sh\n# version 1\n")
	host := unix.New(unix.WithEnv(f.lookup), unix.WithExecutable(func() (string, error) { return binary, nil }))
	first := startRestartableDaemon(t, host)
	waitDaemonFile(t, f, "first")
	f.writeExecutable(t, binary, "#!/bin/sh\n# version 1\n")
	first.assertRunning(t, 2500*time.Millisecond)
	f.writeExecutable(t, binary, "#!/bin/sh\n# version 2 is longer\n")
	if code := first.wait(t, 5*time.Second); code != service.RestartExitCode {
		t.Fatalf("exit status %d, want %d; stderr: %s", code, service.RestartExitCode, first.stderr.String())
	}
	if !strings.Contains(first.stderr.String(), binary) {
		t.Fatalf("stderr does not name the executable: %s", first.stderr.String())
	}
	assertDaemonReleased(t, f)
	second := startRestartableDaemon(t, host)
	waitDaemonLog(t, f, "started", false, 2)
	second.assertRunning(t, 2500*time.Millisecond)
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	waitDaemonLog(t, f, "executable", true, 1)
	second.assertRunning(t, 2500*time.Millisecond)
	second.cancel()
	if code := second.wait(t, 3*time.Second); code != 0 {
		t.Fatalf("second daemon exited %d: %s", code, second.stderr.String())
	}
	if restarts := waitDaemonLog(t, f, "restart", false, 1); len(restarts) != 1 {
		t.Fatalf("restarted %d times", len(restarts))
	}
	assertDaemonReleased(t, f)
}

func (f daemonFixture) writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := f.host.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.host.WriteAtomic(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

type restartableDaemon struct {
	stderr *bytes.Buffer
	cancel context.CancelFunc
	done   chan int
}

func startRestartableDaemon(t *testing.T, host platform.Host) restartableDaemon {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	d := restartableDaemon{stderr: new(bytes.Buffer), cancel: cancel, done: make(chan int, 1)}
	cli := &commandLine{host: host, stdout: io.Discard, stderr: d.stderr, executableCheck: time.Second}
	go func() { d.done <- cli.execute(ctx, []string{"daemon"}) }()
	t.Cleanup(cancel)
	return d
}

func (d restartableDaemon) assertRunning(t *testing.T, period time.Duration) {
	t.Helper()
	select {
	case code := <-d.done:
		t.Fatalf("daemon exited %d: %s", code, d.stderr.String())
	case <-time.After(period):
	}
}

func (d restartableDaemon) wait(t *testing.T, timeout time.Duration) int {
	t.Helper()
	select {
	case code := <-d.done:
		return code
	case <-time.After(timeout):
		t.Fatal("daemon did not exit")
		return -1
	}
}

func TestDaemonCheckWithoutSyncAndStoppedFallback(t *testing.T) {
	t.Parallel()
	f := newDaemonFixture(t)
	command, output := startDaemonProcess(t, f)
	waitDaemonFile(t, f, "first")
	subscriber := subscribeDaemon(t, f)
	waitDaemonEvent(t, subscriber, "poll.finished")
	transport := client.Client{Host: f.host}
	var before workflow.Status
	if err := transport.Call(t.Context(), "status.get", api.Params{}, &before, nil); err != nil {
		t.Fatal(err)
	}
	waitDaemonEvent(t, subscriber, "poll.finished")
	var after workflow.Status
	if err := transport.Call(t.Context(), "status.get", api.Params{}, &after, nil); err != nil {
		t.Fatal(err)
	}
	assertDaemonCheckProgress(t, before, after)
	c := assertVerboseDaemonCheck(t, f)
	stopDaemonProcess(t, f, command, output)
	stopped, err := c.status(t.Context())
	if err != nil || stopped.DaemonRunning || stopped.LastCheck == nil || stopped.LastCheck.At.Before(after.LastCheck.At) {
		t.Fatalf("stopped=%+v err=%v", stopped, err)
	}
}

func assertDaemonCheckProgress(t *testing.T, before, after workflow.Status) {
	t.Helper()
	if before.LastCheck == nil || after.LastCheck == nil || after.LastSync == nil || before.LastSync == nil {
		t.Fatal("missing check/sync")
	}
	if !after.DaemonRunning || after.LastCheck.Outcome != "ok" || !after.LastCheck.At.After(before.LastCheck.At) || !after.LastSync.At.Equal(before.LastSync.At) || len(after.LastCheck.Head) != 12 {
		t.Fatalf("before=%+v after=%+v", before, after)
	}
}

func assertVerboseDaemonCheck(t *testing.T, f daemonFixture) *commandLine {
	t.Helper()
	var diagnostic bytes.Buffer
	c := &commandLine{host: f.host, stdout: io.Discard, stderr: &diagnostic, flags: globalFlags{verbose: true}}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(t.Context())
	if err := c.prepare(cmd, nil); err != nil {
		t.Fatal(err)
	}
	live, err := c.status(t.Context())
	if err != nil || !live.DaemonRunning || live.LastCheck == nil || !strings.Contains(diagnostic.String(), "working copy") {
		t.Fatalf("verbose=%+v err=%v diagnostics=%s", live, err, diagnostic.String())
	}
	return c
}
