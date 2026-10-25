package config_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

func TestWorkingCopyLocations(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	for _, tc := range []struct{ path, want string }{
		{"", filepath.Join(home, ".local", "share", "kitbag", "repos", "agent-config")},
		{"~/src/config", filepath.Join(home, "src", "config")},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			c, err := config.Parse([]byte("schema=1\n[repo]\nurl='git@host:me/agent-config.git'"))
			if err != nil {
				t.Fatal(err)
			}
			c.Repo.Path = tc.path
			got, err := c.WorkingCopy(h)
			if err != nil || got != tc.want {
				t.Fatalf("location: %q, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestLoadMissingAndExplicit(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	h := fake.New(fake.WithEnv(map[string]string{"HOME": home}))
	c, err := config.Load(h, "")
	if err != nil || c.Schema != 1 {
		t.Fatalf("absent default: %+v, %v", c, err)
	}
	if _, err = c.WorkingCopy(h); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("absent repository: %v", err)
	}
	filename := filepath.Join(home, "machine.toml")
	if _, err = config.Load(h, filename); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("explicit missing: %v", err)
	}
	if err = h.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = h.WriteAtomic(filename, []byte("schema=1\n[repo]\nurl='local'"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = config.Load(h, filename); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyOptionalValuesRejected(t *testing.T) {
	t.Parallel()
	for _, extra := range []string{"host=''\n", "[git]\nbackend=''", "[sync]\nmode=''", "[repo]\nurl='local'\npath=''"} {
		t.Run(extra, func(t *testing.T) {
			t.Parallel()
			body := "schema=1\n" + extra
			if !strings.Contains(extra, "[repo]") {
				body += "\n[repo]\nurl='local'"
			}
			if _, err := config.Parse([]byte(body)); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("empty optional accepted: %v", err)
			}
		})
	}
}

func TestCommitIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		identity string
		valid    bool
	}{
		{"name='Publisher'\nemail='publisher@example.com'", true},
		{"name='Publisher'", false},
		{"email='publisher@example.com'", false},
		{"name='Publisher'\nemail='not-an-email'", false},
		{"name='Publisher <Other>'\nemail='publisher@example.com'", false},
	} {
		t.Run(tc.identity, func(t *testing.T) {
			t.Parallel()
			c, err := config.Parse([]byte("schema=1\n[repo]\nurl='local'\n[git.identity]\n" + tc.identity))
			if (err == nil) != tc.valid {
				t.Fatalf("parse identity %+v, %v; valid %v", c.Git.Identity, err, tc.valid)
			}
			if tc.valid && (c.Git.Identity.Name != "Publisher" || c.Git.Identity.Email != "publisher@example.com") {
				t.Fatalf("identity %+v", c.Git.Identity)
			}
		})
	}
}
