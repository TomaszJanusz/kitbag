package discover

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

func home() string { return filepath.FromSlash("/home/discover") }

// table mirrors the adapters' rules: an absolute override (Claude), a parent
// override (Gemini), an XDG base that ignores relative values (OpenCode) and
// a kind without a default (generic).
func table() Table {
	return Table{
		{Agent: "claude", Variable: "CLAUDE_CONFIG_DIR", Home: func(env platform.Env) (string, error) {
			return env.AgentHome("CLAUDE_CONFIG_DIR", ".claude")
		}},
		{Agent: "gemini", Variable: "GEMINI_CLI_HOME", Home: func(env platform.Env) (string, error) {
			if parent, _ := env.LookupEnv("GEMINI_CLI_HOME"); parent != "" {
				return filepath.Join(parent, ".gemini"), nil
			}
			return env.AgentHome("", ".gemini")
		}},
		{Agent: "opencode", Variable: "XDG_CONFIG_HOME", Home: func(env platform.Env) (string, error) {
			dirs, err := env.Dirs()
			return filepath.Join(filepath.Dir(dirs.Config), "opencode"), err
		}},
		{Agent: "generic"},
	}
}

// recorder fails the test on every host call that could read or list
// content, or follow a link: discovery may only resolve paths and Lstat.
type recorder struct {
	platform.Host
	t *testing.T
}

func (r recorder) ReadFile(path string) ([]byte, error) {
	r.t.Errorf("discovery read %s", path)
	return nil, fs.ErrPermission
}

func (r recorder) ReadDir(path string) ([]fs.DirEntry, error) {
	r.t.Errorf("discovery listed %s", path)
	return nil, fs.ErrPermission
}

func (r recorder) Stat(path string) (fs.FileInfo, error) {
	r.t.Errorf("discovery followed %s", path)
	return nil, fs.ErrPermission
}

func (r recorder) ReadLink(path string) (string, error) {
	r.t.Errorf("discovery read link %s", path)
	return "", fs.ErrPermission
}

func newHost(env map[string]string) *fake.Host {
	vars := map[string]string{"HOME": home()}
	maps.Copy(vars, env)
	return fake.New(fake.WithEnv(vars))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsSourcesAndPresence(t *testing.T) {
	t.Parallel()
	outside := filepath.FromSlash("/srv/claude")
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": outside, "GEMINI_CLI_HOME": home(), "XDG_CONFIG_HOME": "relative"})
	must(t, host.MkdirAll(outside, dirPerm))
	must(t, host.MkdirAll(filepath.Join(home(), ".config"), dirPerm))
	// Credentials beside and inside candidates must never be read.
	must(t, host.WriteAtomic(filepath.Join(outside, ".credentials.json"), []byte("secret"), filePerm))
	must(t, host.WriteAtomic(filepath.Join(home(), ".gemini"), []byte("file"), filePerm))
	must(t, host.MkdirAll(filepath.Join(home(), "elsewhere"), dirPerm))
	must(t, host.WriteAtomic(filepath.Join(home(), "elsewhere", "auth.json"), []byte("secret"), filePerm))
	must(t, host.Link(filepath.Join(home(), "elsewhere"), filepath.Join(home(), ".config", "opencode")))
	got, err := table().Defaults(recorder{host, t})
	must(t, err)
	want := []Candidate{
		{Agent: "claude", Directory: outside, Source: Environment, Variable: "CLAUDE_CONFIG_DIR", Presence: Present},
		{Agent: "gemini", Directory: filepath.Join(home(), ".gemini"), Source: Environment, Variable: "GEMINI_CLI_HOME", Presence: NotDirectory},
		// A relative XDG base is ignored, so the default applies.
		{Agent: "opencode", Directory: filepath.Join(home(), ".config", "opencode"), Source: Default, Presence: Symlink},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates\n got %+v\nwant %+v", got, want)
	}
}

func TestDefaultsUnsetVariablesAndMissing(t *testing.T) {
	t.Parallel()
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": ""})
	got, err := table().Defaults(recorder{host, t})
	must(t, err)
	want := []Candidate{
		{Agent: "claude", Directory: filepath.Join(home(), ".claude"), Source: Default, Presence: Missing},
		{Agent: "gemini", Directory: filepath.Join(home(), ".gemini"), Source: Default, Presence: Missing},
		{Agent: "opencode", Directory: filepath.Join(home(), ".config", "opencode"), Source: Default, Presence: Missing},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("candidates\n got %+v\nwant %+v", got, want)
	}
}

func TestDefaultsRelativeOverrideFails(t *testing.T) {
	t.Parallel()
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": "relative"})
	if got, err := table().Defaults(recorder{host, t}); !errors.Is(err, fs.ErrInvalid) || got != nil {
		t.Fatalf("relative override: %+v %v", got, err)
	}
}

type lstatError struct {
	platform.Host
	err error
}

func (h lstatError) Lstat(string) (fs.FileInfo, error) { return nil, h.err }

func TestDefaultsUnreadable(t *testing.T) {
	t.Parallel()
	host := lstatError{newHost(nil), fs.ErrPermission}
	got, err := table().Only("claude").Defaults(host)
	must(t, err)
	if len(got) != 1 || got[0].Presence != Unreadable || !errors.Is(got[0].Err, fs.ErrPermission) {
		t.Fatalf("unreadable: %+v", got)
	}
}

func TestHomeAndOnly(t *testing.T) {
	t.Parallel()
	host := newHost(map[string]string{"CLAUDE_CONFIG_DIR": filepath.FromSlash("/srv/claude")})
	homes := table()
	if dir, err := homes.Home(host, "claude"); dir != filepath.FromSlash("/srv/claude") || err != nil {
		t.Fatalf("claude: %q %v", dir, err)
	}
	if dir, err := homes.Home(host, "generic"); dir != "" || err != nil {
		t.Fatalf("generic: %q %v", dir, err)
	}
	if dir, err := homes.Home(host, "cursor"); !errors.Is(err, ErrUnsupported) || err.Error() != "unsupported agent cursor" {
		t.Fatalf("cursor: %q %v", dir, err)
	}
}

func TestOnlyKeepsTableOrder(t *testing.T) {
	t.Parallel()
	homes := table()
	only := homes.Only("opencode", "claude", "missing")
	if len(only) != 2 || only[0].Agent != "claude" || only[1].Agent != "opencode" || len(homes) != 4 {
		t.Fatalf("only: %+v (table %d)", only, len(homes))
	}
}
