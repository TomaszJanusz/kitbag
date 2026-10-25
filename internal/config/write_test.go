package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func initialConfig() Config {
	c := Config{Schema: 1}
	c.Repo.URL = "https://example.com/config.git"
	c.Git.Backend, c.Sync.Mode = "embedded", "copy"
	return c
}

func TestEncodeAndCreate(t *testing.T) {
	t.Parallel()
	c := initialConfig()
	c.Git.Identity.Name, c.Git.Identity.Email = "Name with 'quotes'", "name@example.com"
	c.Host = "runner"
	data, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil || parsed.Git.Identity != c.Git.Identity {
		t.Fatalf("round trip: %+v %v", parsed, err)
	}
	home := filepath.Join(string(filepath.Separator), "home", "test")
	host := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	filename := filepath.Join(home, "config", "config.toml")
	if err = Create(host, filename, c); err != nil {
		t.Fatal(err)
	}
	info, err := host.Lstat(filename)
	if err != nil || info.Mode().Perm() != configFilePerm {
		t.Fatalf("mode: %v %v", info, err)
	}
	if err = Create(host, filename, c); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("clobber: %v", err)
	}
	if _, err = host.Lstat(filename + ".init-new"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("staging leak: %v", err)
	}
}

func TestCreatePreservesExistingSymlink(t *testing.T) {
	t.Parallel()
	host := fake.New()
	filename := filepath.Join(string(filepath.Separator), "config.toml")
	if err := host.Link("missing", filename); err != nil {
		t.Fatal(err)
	}
	if err := Create(host, filename, initialConfig()); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("clobbered link: %v", err)
	}
	if target, err := host.ReadLink(filename); err != nil || target != "missing" {
		t.Fatalf("link: %s %v", target, err)
	}
}

func TestCreateInterruptedAndOccupiedStage(t *testing.T) {
	t.Parallel()
	host := fake.New()
	filename := filepath.Join(string(filepath.Separator), "config.toml")
	if err := host.InterruptNextWrite(filename+".init-new", fake.BeforeRename); err != nil {
		t.Fatal(err)
	}
	if err := Create(host, filename, initialConfig()); err == nil {
		t.Fatal("interrupted write succeeded")
	}
	if _, err := host.Lstat(filename); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("partial config: %v", err)
	}
	if err := host.WriteAtomic(filename+".init-new", []byte("another writer"), configFilePerm); err != nil {
		t.Fatal(err)
	}
	if err := Create(host, filename, initialConfig()); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("overwrote occupied stage: %v", err)
	}
}
