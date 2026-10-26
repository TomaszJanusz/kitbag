//go:build !windows

package overlay_test

import (
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
	"github.com/TomaszJanusz/kitbag/internal/platform/overlay"
)

func TestEnvironmentReplacesOnlyListedKeys(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "u")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home, "CLAUDE_CONFIG_DIR": "/daemon/claude", "CODEX_HOME": "/daemon/codex", "PATH": "/bin"}))
	keys := []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_CONFIG_HOME"}
	h := overlay.Environment(host, keys, map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(home, ".claude_2"), "XDG_CONFIG_HOME": filepath.Join(home, "cfg")})

	if value, ok := h.LookupEnv("CLAUDE_CONFIG_DIR"); !ok || value != filepath.Join(home, ".claude_2") {
		t.Fatalf("overlaid variable: %q %v", value, ok)
	}
	if _, ok := h.LookupEnv("CODEX_HOME"); ok {
		t.Fatal("a listed key the caller did not send must be unset, not the daemon's")
	}
	if value, _ := h.LookupEnv("PATH"); value != "/bin" {
		t.Fatalf("unlisted variable: %q", value)
	}
	checkHomes(t, h, home)
}

func checkHomes(t *testing.T, h platform.Env, home string) {
	t.Helper()
	if dir, err := h.AgentHome("CLAUDE_CONFIG_DIR", ".claude"); err != nil || dir != filepath.Join(home, ".claude_2") {
		t.Fatalf("relocated home: %q %v", dir, err)
	}
	if dir, err := h.AgentHome("CODEX_HOME", ".codex"); err != nil || dir != filepath.Join(home, ".codex") {
		t.Fatalf("default home: %q %v", dir, err)
	}
	dirs, err := h.Dirs()
	if err != nil || dirs.Config != filepath.Join(home, "cfg", "kitbag") {
		t.Fatalf("dirs: %+v %v", dirs, err)
	}
}

func TestEnvironmentWithoutKeysIsTheHost(t *testing.T) {
	t.Parallel()
	host := fake.New()
	if overlay.Environment(host, nil, map[string]string{"HOME": "/x"}) != host {
		t.Fatal("no keys must return the host itself")
	}
}
