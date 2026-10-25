package system

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// fakeRunner finds git only in gitDir and answers every command with reply.
type fakeRunner struct {
	gitDir string
	reply  func(cmd Command) Result
	cmds   []Command
}

func (r *fakeRunner) LookPath(name, pathList string) (string, error) {
	if slices.Contains(strings.Split(pathList, ":"), r.gitDir) {
		return r.gitDir + "/" + name, nil
	}
	return "", exec.ErrNotFound
}

func (r *fakeRunner) Run(_ context.Context, cmd Command) (Result, error) {
	r.cmds = append(r.cmds, cmd)
	return r.reply(cmd), nil
}

func versionReply(version string) func(Command) Result {
	return func(cmd Command) Result {
		if slices.Equal(cmd.Args, []string{"--version"}) {
			return Result{Stdout: []byte(version)}
		}
		return Result{}
	}
}

// hostileEnv is a platform environment full of variables that must not
// reach Git.
func hostileEnv(path string) map[string]string {
	return map[string]string{ //nolint:gosec // G101: askpass variable names, no credentials.
		"HOME":                "/home/me",
		"PATH":                path,
		"SSH_AUTH_SOCK":       "/tmp/agent.sock",
		"GIT_DIR":             "/elsewhere/.git",
		"GIT_WORK_TREE":       "/elsewhere",
		"GIT_INDEX_FILE":      "/elsewhere/index",
		"GIT_CONFIG_GLOBAL":   "/elsewhere/config",
		"GIT_CONFIG_COUNT":    "1",
		"GIT_CONFIG_KEY_0":    "core.pager",
		"GIT_CONFIG_VALUE_0":  "less",
		"GIT_EXEC_PATH":       "/elsewhere/libexec",
		"GIT_SSH_COMMAND":     "ssh -o StrictHostKeyChecking=no",
		"GIT_ASKPASS":         "/usr/bin/askpass",
		"SSH_ASKPASS":         "/usr/bin/askpass",
		"EDITOR":              "vim",
		"LANG":                "de_DE.UTF-8",
		"KITBAG_GIT_BACKEND":  "system",
		"XDG_CONFIG_HOME":     "/home/me/.config",
		"GIT_TERMINAL_PROMPT": "1",
	}
}

func TestNewEnvironmentIsExplicit(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{gitDir: "/opt/git/bin", reply: versionReply("git version 2.43.0\n")}
	env := fake.New(fake.WithEnv(hostileEnv("/usr/bin:/opt/git/bin")))
	b, err := New(t.Context(), Options{Env: env, Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	if b.Path() != "/opt/git/bin/git" || b.Version() != "git 2.43.0" || b.Kind() != vcs.System {
		t.Errorf("backend = %s %q %s", b.Path(), b.Version(), b.Kind())
	}
	if _, err := b.Status(t.Context(), "/work/repo"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"HOME=/home/me", "PATH=/usr/bin:/opt/git/bin", "XDG_CONFIG_HOME=/home/me/.config",
		"SSH_AUTH_SOCK=/tmp/agent.sock", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=false",
		"GIT_SEQUENCE_EDITOR=false", "GIT_PAGER=cat", "PAGER=cat",
	}
	if got := r.cmds[0].Env; !slices.Equal(got, want) {
		t.Errorf("version env = %q\nwant          %q", got, want)
	}
	status := r.cmds[1]
	wantRepo := append(slices.Clone(want), "GIT_CEILING_DIRECTORIES=/work")
	if !slices.Equal(status.Env, wantRepo) || status.Dir != "/work/repo" || status.Path != "/opt/git/bin/git" {
		t.Errorf("status command = %+v\nwant env %q in /work/repo", status, wantRepo)
	}
}

func TestNewNoSystemConfig(t *testing.T) {
	t.Parallel()

	r := &fakeRunner{gitDir: "/bin", reply: versionReply("git version 2.30.0\n")}
	env := fake.New(fake.WithEnv(map[string]string{"HOME": "/h", "PATH": "/bin"}))
	if _, err := New(t.Context(), Options{Env: env, Runner: r, NoSystemConfig: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.cmds[0].Env, "GIT_CONFIG_NOSYSTEM=1") {
		t.Errorf("env = %q, want GIT_CONFIG_NOSYSTEM=1", r.cmds[0].Env)
	}
}

func TestNewFailures(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		want    error
		env     map[string]string
		name    string
		version string
	}{
		{ErrGitNotFound, map[string]string{"HOME": "/h", "PATH": "/usr/bin"}, "no git on PATH", ""},
		{ErrGitNotFound, map[string]string{"HOME": "/h"}, "no PATH", ""},
		{ErrGitTooOld, map[string]string{"HOME": "/h", "PATH": "/bin"}, "too old", "git version 2.29.2\n"},
		{ErrUnknownVersion, map[string]string{"HOME": "/h", "PATH": "/bin"}, "not git", "hello\n"},
		{nil, map[string]string{"PATH": "/bin"}, "no HOME", "git version 2.43.0\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &fakeRunner{gitDir: "/bin", reply: versionReply(tt.version)}
			_, err := New(t.Context(), Options{Env: fake.New(fake.WithEnv(tt.env)), Runner: r})
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Fatalf("New error = %v, want %v", err, tt.want)
			}
		})
	}

	failing := &fakeRunner{gitDir: "/bin", reply: func(Command) Result { return Result{ExitCode: 1, Stderr: []byte("boom\n")} }}
	env := fake.New(fake.WithEnv(map[string]string{"HOME": "/h", "PATH": "/bin"}))
	var cmdErr *CommandError
	if _, err := New(t.Context(), Options{Env: env, Runner: failing}); !errors.As(err, &cmdErr) || cmdErr.Stderr != "boom" {
		t.Errorf("failing git --version: error = %v, want CommandError with stderr", err)
	}
	if _, err := New(t.Context(), Options{}); !errors.Is(err, vcs.ErrInvalidArgument) {
		t.Errorf("New without environment: error = %v", err)
	}
}

func TestCommandError(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		want string
		err  CommandError
	}{
		{"git fetch: exit status 128: fatal: no", CommandError{Args: []string{"fetch", "origin"}, ExitCode: 128, Stderr: "fatal: no"}},
		{"git push: exit status 1", CommandError{Args: []string{"push"}, ExitCode: 1}},
		{"git: exit status 2", CommandError{ExitCode: 2}},
	} {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}
