package system

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

func TestGlobalConfigEnvironmentAndCommands(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{gitDir: "/bin", reply: versionReply("git version 2.43.0\n")}
	b, err := New(t.Context(), Options{Env: fake.New(fake.WithEnv(hostileEnv("/bin"))), Runner: r, NoSystemConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	r.reply = func(Command) Result { return Result{Stdout: []byte("~/a\x00/path with spaces\x00")} }
	got, err := b.GlobalIncludes(t.Context())
	if err != nil || !slices.Equal(got, []string{"~/a", "/path with spaces"}) {
		t.Fatalf("includes = %q, %v", got, err)
	}
	if err := b.AddGlobalInclude(t.Context(), "/home/me/repo/config"); err != nil {
		t.Fatal(err)
	}
	if got := b.GlobalConfigFiles(); !slices.Equal(got, []string{"/elsewhere/config"}) {
		t.Fatalf("files = %v", got)
	}
	wantRead := []string{"config", "--global", "--no-includes", "--null", "--get-all", "include.path"}
	wantAdd := []string{"config", "--global", "--add", "include.path", "/home/me/repo/config"}
	if !slices.Equal(r.cmds[1].Args, wantRead) || !slices.Equal(r.cmds[2].Args, wantAdd) {
		t.Fatalf("commands = %+v", r.cmds)
	}
	for _, cmd := range r.cmds[1:] {
		assertGlobalEnvironment(t, cmd)
	}
}

func assertGlobalEnvironment(t *testing.T, cmd Command) {
	t.Helper()
	if cmd.Dir != "/" || !slices.Contains(cmd.Env, "GIT_CONFIG_GLOBAL=/elsewhere/config") || !slices.Contains(cmd.Env, "GIT_CONFIG_NOSYSTEM=1") {
		t.Fatalf("global command = %+v", cmd)
	}
	for _, kv := range cmd.Env {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "GIT_") && !slices.Contains([]string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_TERMINAL_PROMPT", "GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "GIT_PAGER"}, key) {
			t.Fatalf("leaked environment: %s", key)
		}
	}
}

func TestGlobalConfigFiles(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, xdg, global string
		want              []string
	}{
		{name: "default", want: []string{"/home/me/.gitconfig", "/home/me/.config/git/config"}},
		{name: "xdg", xdg: "/xdg", want: []string{"/home/me/.gitconfig", "/xdg/git/config"}},
		{name: "override", global: "/custom/config", want: []string{"/custom/config"}},
		{name: "relative override", global: "relative"},
		{name: "unclean override", global: "/config/../global"},
		{name: "relative xdg", xdg: "relative"},
		{name: "unclean xdg", xdg: "/config/../xdg"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{"HOME": "/home/me", "PATH": "/bin", "XDG_CONFIG_HOME": tt.xdg, "GIT_CONFIG_GLOBAL": tt.global}
			r := &fakeRunner{gitDir: "/bin", reply: versionReply("git version 2.43.0\n")}
			b, err := New(t.Context(), Options{Env: fake.New(fake.WithEnv(env)), Runner: r})
			if err != nil {
				t.Fatal(err)
			}
			if got := b.GlobalConfigFiles(); !slices.Equal(got, tt.want) {
				t.Fatalf("files = %v, want %v", got, tt.want)
			}
			if tt.want == nil {
				_, err = b.GlobalIncludes(t.Context())
				if !errors.Is(err, vcs.ErrInvalidArgument) || len(r.cmds) != 1 {
					t.Fatalf("invalid config path error = %v, commands = %v", err, r.cmds)
				}
			}
		})
	}
}

func TestGlobalConfigCommandErrors(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{gitDir: "/bin", reply: versionReply("git version 2.43.0\n")}
	b, err := New(t.Context(), Options{Env: fake.New(fake.WithEnv(map[string]string{"HOME": "/home/me", "PATH": "/bin"})), Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	r.reply = func(Command) Result { return Result{ExitCode: 1} }
	if includes, readErr := b.GlobalIncludes(t.Context()); readErr != nil || len(includes) != 0 {
		t.Fatalf("absent key = %v, %v", includes, readErr)
	}
	r.reply = func(Command) Result { return Result{ExitCode: 128, Stderr: []byte("bad config")} }
	var cmdErr *CommandError
	if _, readErr := b.GlobalIncludes(t.Context()); !errors.As(readErr, &cmdErr) {
		t.Fatalf("read error = %v", readErr)
	}
	if addErr := b.AddGlobalInclude(t.Context(), "/home/me/include"); !errors.As(addErr, &cmdErr) {
		t.Fatalf("add error = %v", addErr)
	}
	before := len(r.cmds)
	if addErr := b.AddGlobalInclude(t.Context(), "relative"); !errors.Is(addErr, vcs.ErrInvalidArgument) || len(r.cmds) != before {
		t.Fatalf("invalid add = %v", addErr)
	}
}
