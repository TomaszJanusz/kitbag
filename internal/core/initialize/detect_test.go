package initialize

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

type directoryOnly struct {
	platform.Host
	reads *int
}

func (h directoryOnly) ReadFile(string) ([]byte, error) { *h.reads++; return nil, fs.ErrPermission }
func (h directoryOnly) ReadDir(string) ([]fs.DirEntry, error) {
	*h.reads++
	return nil, fs.ErrPermission
}

// homes mirrors the adapters' resolvers that cmd/kitbag injects, plus a kind
// init cannot scaffold whose relative override would fail resolution.
func homes() discover.Table {
	agentHome := func(variable, rel string) func(platform.Env) (string, error) {
		return func(env platform.Env) (string, error) { return env.AgentHome(variable, rel) }
	}
	return discover.Table{
		{Agent: "gemini", Variable: "GEMINI_CLI_HOME", Home: agentHome("GEMINI_CLI_HOME", ".gemini")},
		{Agent: "claude", Variable: "CLAUDE_CONFIG_DIR", Home: agentHome("CLAUDE_CONFIG_DIR", ".claude")},
		{Agent: "codex", Variable: "CODEX_HOME", Home: agentHome("CODEX_HOME", ".codex")},
		{Agent: "generic"},
	}
}

func write(t *testing.T, host platform.FS, path, body string) {
	t.Helper()
	if err := host.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := host.WriteAtomic(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectionNeverReadsContents(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	relocated := filepath.Join(home, "claude-work")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home, "CLAUDE_CONFIG_DIR": relocated, "GEMINI_CLI_HOME": "relative"}))
	for _, dir := range []string{relocated, filepath.Join(home, ".codex"), filepath.Join(home, ".claude_2"), filepath.Join(home, ".gemini")} {
		write(t, host, filepath.Join(dir, "secret"), "never read")
	}
	reads := 0
	got, err := Detect(directoryOnly{Host: host, reads: &reads}, homes())
	want := []Agent{{"claude", relocated}, {"codex", filepath.Join(home, ".codex")}}
	if err != nil || !slices.Equal(got, want) || reads != 0 {
		t.Fatalf("detection %+v %v reads=%d", got, err, reads)
	}
}

func TestDetectionMissingFileAndSymlink(t *testing.T) {
	t.Parallel()
	s, _ := setup(t)
	got, err := Detect(s.Host, homes())
	if err != nil || len(got) != 0 {
		t.Fatalf("missing: %+v %v", got, err)
	}
	home, err := s.Host.HomeDir()
	must(t, err)
	write(t, s.Host, filepath.Join(home, ".claude"), "file")
	err = s.Host.Link(filepath.Join(home, "elsewhere"), filepath.Join(home, ".codex"))
	must(t, err)
	got, err = Detect(s.Host, homes())
	if err != nil || len(got) != 0 {
		t.Fatalf("unsafe dirs: %+v %v", got, err)
	}
}

func TestDetectionFailsOnInvalidOverrideOrLstatError(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home, "CODEX_HOME": "relative"}))
	if got, err := Detect(host, homes()); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("relative CODEX_HOME: %+v %v", got, err)
	}
	host = fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	if got, err := Detect(host, homes()); err != nil || len(got) != 0 {
		t.Fatalf("baseline: %+v %v", got, err)
	}
	broken := lstatError{Host: host, err: fs.ErrPermission}
	if got, err := Detect(broken, homes()); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("lstat error: %+v %v", got, err)
	}
}

type lstatError struct {
	platform.Host
	err error
}

func (h lstatError) Lstat(string) (fs.FileInfo, error) { return nil, h.err }

func TestConfigurationPathsAndExistingEntries(t *testing.T) {
	t.Parallel()
	s, _ := setup(t)
	explicit := filepath.Join(string(filepath.Separator), "explicit", "config.toml")
	if path, err := ConfigPath(s.Host, explicit); path != explicit || err != nil {
		t.Fatalf("flag: %s %v", path, err)
	}
	host := fake.New(fake.WithEnv(map[string]string{"HOME": "/home/test", "KITBAG_CONFIG": explicit}))
	if path, err := ConfigPath(host, ""); path != explicit || err != nil {
		t.Fatalf("env: %s %v", path, err)
	}
}

func TestExistingEntriesArePreserved(t *testing.T) {
	t.Parallel()
	s, _ := setup(t)
	if exists, err := Exists(s.Host, s.Filename); exists || err != nil {
		t.Fatalf("missing: %t %v", exists, err)
	}
	write(t, s.Host, s.Filename, "invalid existing configuration")
	if exists, err := Exists(s.Host, s.Filename); !exists || err != nil {
		t.Fatalf("existing: %t %v", exists, err)
	}
	if err := s.Save(); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("replaced config: %v", err)
	}
	data, err := s.Host.ReadFile(s.Filename)
	if err != nil || string(data) != "invalid existing configuration" {
		t.Fatalf("changed config: %q %v", data, err)
	}
}
