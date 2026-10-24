//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

// TestStateProcesses is the real-host integration exception: all state unit
// tests use fake. Pipes force overlap without sleeps or scheduler assumptions.
func TestStateProcesses(t *testing.T) {
	t.Parallel()
	for _, crash := range []bool{false, true} {
		t.Run(fmt.Sprintf("crash=%t", crash), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			holder, input, output := startStateHolder(t, root)
			line, err := output.ReadString('\n')
			if err != nil || line != "READY\n" {
				t.Fatalf("holder readiness = %q, %v", line, err)
			}
			contender := stateCommand(t, root, "contend")
			result, err := contender.CombinedOutput()
			if err != nil || !bytes.Contains(result, []byte("CONTENDED")) {
				t.Fatalf("contender = %s, %v", result, err)
			}
			finishStateHolder(t, holder, input, crash)
			assertProcessSnapshot(t, root, crash)
		})
	}
}

func stateCommand(t *testing.T, root, mode string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const timeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return exec.CommandContext(ctx, executable, "-test.run=^TestStateProcessHelper$", "--", root, mode) //nolint:gosec // G204: re-exec this test binary, with arguments from this test; no shell.
}

func startStateHolder(t *testing.T, root string) (*exec.Cmd, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	cmd := stateCommand(t, root, "hold")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Logf("holder cleanup: %v, %s", err, stderr.String())
			}
		}
		if err := input.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
	})
	return cmd, input, bufio.NewReader(output)
}

func realStateStore(t *testing.T, root string) *state.Store {
	t.Helper()
	host := unix.New(unix.WithEnv(func(key string) (string, bool) {
		if key == "HOME" || key == "XDG_STATE_HOME" {
			return root, true
		}
		return "", false
	}))
	store, err := state.New(host, host, state.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// TestStateProcessHelper runs only when re-executed with explicit arguments.
func TestStateProcessHelper(t *testing.T) {
	t.Parallel()
	index := slices.Index(os.Args, "--")
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	if len(args) != 2 {
		t.Fatal("expected root and mode")
	}
	store := realStateStore(t, args[0])
	if args[1] == "contend" {
		assertStateContention(t, store)
		return
	}
	if err := store.WithLock(func(session *state.Session) error {
		if err := saveProcessState(session, "before"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
			return err
		}
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			return err
		}
		return saveProcessState(session, "complete")
	}); err != nil {
		t.Fatal(err)
	}
}

func saveProcessState(session *state.Session, id string) error {
	return session.Save(state.Database{SchemaVersion: state.CurrentVersion, History: []state.AppliedPlan{{ID: id, AppliedAt: time.Now()}}})
}

func finishStateHolder(t *testing.T, holder *exec.Cmd, input io.Writer, crash bool) {
	t.Helper()
	if crash {
		if err := holder.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		if err := holder.Wait(); err == nil {
			t.Fatal("killed holder succeeded")
		}
		return
	}
	if _, err := io.WriteString(input, "finish\n"); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatal(err)
	}
}

func assertProcessSnapshot(t *testing.T, root string, crash bool) {
	t.Helper()
	store := realStateStore(t, root)
	if err := store.WithLock(func(session *state.Session) error {
		db, err := session.Load()
		if err != nil {
			return err
		}
		want := "complete"
		if crash {
			want = "before"
		}
		if len(db.History) != 1 || db.History[0].ID != want {
			t.Fatalf("state corrupted/interleaved: %+v", db)
		}
		return session.Save(db) // Reacquires unchanged lock file after death/exit.
	}); err != nil {
		t.Fatal(err)
	}
}

func assertStateContention(t *testing.T, store *state.Store) {
	t.Helper()
	err := store.WithLock(func(*state.Session) error { t.Error("second process entered locked operation"); return nil })
	if !errors.Is(err, platform.ErrLocked) || !strings.Contains(err.Error(), "another sync") {
		t.Fatalf("contention = %v", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "CONTENDED"); err != nil {
		t.Fatal(err)
	}
}
