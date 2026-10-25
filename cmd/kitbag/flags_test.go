package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestConfigBackendAndGlobalFlags(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	filename := filepath.Join(home, "machine.toml")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home, "PATH": "", "KITBAG_CONFIG": filename}))
	if err := h.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteAtomic(filename, []byte("schema=1\n[repo]\nurl='local'\n[git]\nbackend='system'"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run("linux", h, []string{"version"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("selected by git.backend in config.toml")) {
		t.Fatalf("config backend ignored: %s", &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run("linux", h, []string{"version", "--git-backend", "auto"}, &stdout, &stderr); code != 0 {
		t.Fatalf("override: %d, %s", code, &stderr)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run("linux", h, []string{"version", "--verbose", "--quiet"}, &stdout, &stderr); code != 2 {
		t.Fatalf("mutually exclusive flags: %d", code)
	}
}
