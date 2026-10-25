package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

// noGit is an environment whose PATH holds no git.
func noGit(extra ...string) platform.Env {
	env := map[string]string{"HOME": "/home/me", "PATH": ""}
	for i := 0; i+1 < len(extra); i += 2 {
		env[extra[i]] = extra[i+1]
	}
	return fake.New(fake.WithEnv(env))
}

func TestRun(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		out  string
		err  string
		args []string
		code int
	}{
		{name: "version", args: []string{"version"}, out: "kitbag dev\n"},
		{name: "missing command", code: 2, err: usage + "\n"},
		{name: "unknown command", args: []string{"sync"}, code: 2, err: usage + "\n"},
		{name: "extra argument", args: []string{"version", "extra"}, code: 2, err: usage + "\n"},
		{name: "flag is not a command", args: []string{"--version"}, code: 2, err: usage + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run("linux", noGit(), tt.args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if got := stdout.String(); got != tt.out {
				t.Errorf("stdout = %q, want %q", got, tt.out)
			}
			if got := stderr.String(); got != tt.err {
				t.Errorf("stderr = %q, want %q", got, tt.err)
			}
		})
	}
}

// TestRunRefusesWindows pins the documented message and exit code; changing
// either is a user-visible change that must update docs/platform.md.
func TestRunRefusesWindows(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"version"}, {"sync"}} {
		var stdout, stderr bytes.Buffer
		if code := run("windows", noGit(), args, &stdout, &stderr); code != 5 {
			t.Errorf("args %q: exit code = %d, want 5", args, code)
		}
		if got := stdout.String(); got != "" {
			t.Errorf("args %q: stdout = %q, want empty", args, got)
		}
		if got, want := stderr.String(), "kitbag: Windows is not supported before 1.x\n"; got != want {
			t.Errorf("args %q: stderr = %q, want %q", args, got, want)
		}
	}
}

func TestRunOutputFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if code := run("linux", noGit(), []string{"version"}, failedWriter{}, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "output unavailable") {
		t.Errorf("stderr = %q, want output failure", stderr.String())
	}
}

func TestRunDiagnosticFailure(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		goos string
		args []string
	}{
		{name: "usage diagnostic", goos: "linux"},
		{name: "output failure diagnostic", goos: "darwin", args: []string{"version"}},
		{name: "unsupported OS diagnostic", goos: "windows", args: []string{"version"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if code := run(tt.goos, noGit(), tt.args, failedWriter{}, failedWriter{}); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
		})
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}
