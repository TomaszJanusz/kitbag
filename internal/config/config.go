// Package config reads machine-local configuration through platform contracts.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Config is schema 1 machine intent, independent of the repository manifest.
type Config struct {
	Host string `toml:"host"`
	Repo struct {
		URL    string `toml:"url"`
		Branch string `toml:"branch"`
		Path   string `toml:"path"`
	} `toml:"repo"`
	Git struct {
		Backend string `toml:"backend"`
	} `toml:"git"`
	Sync struct {
		PollInterval string `toml:"poll_interval"`
		Mode         string `toml:"mode"`
	} `toml:"sync"`
	UI struct {
		Notifications *bool  `toml:"notifications"`
		Theme         string `toml:"theme"`
		Color         string `toml:"color"`
	} `toml:"ui"`
	Migration struct {
		LegacyRoots []string `toml:"legacy_roots"`
	} `toml:"migration"`
	Schema int `toml:"schema"`
}

// Load reads the selected file. Only an absent default file is optional.
func Load(host platform.Host, filename string) (Config, error) {
	explicit := filename != ""
	if filename == "" {
		dirs, err := host.Dirs()
		if err != nil {
			return Config{}, err
		}
		filename = filepath.Join(dirs.Config, "config.toml")
	}
	data, err := host.ReadFile(filename)
	if !explicit && errors.Is(err, fs.ErrNotExist) {
		return Config{Schema: 1}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("configuration: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("configuration %s: %w", filename, err)
	}
	return c, nil
}

// Parse rejects unknown fields and unsupported schemas without accessing a host.
func Parse(data []byte) (Config, error) {
	var c Config
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&c); err != nil {
		return c, fmt.Errorf("%w: %w", fs.ErrInvalid, err)
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func invalid(key string) error            { return fmt.Errorf("invalid configuration %s: %w", key, fs.ErrInvalid) }
func match(pattern, value string) bool    { return regexp.MustCompile(pattern).MatchString(value) }
func optional(pattern, value string) bool { return value == "" || match(pattern, value) }

func (c Config) validate() error {
	if c.Schema != 1 {
		return invalid("schema (supported: 1)")
	}
	if !match(`^[^\s]+$`, c.Repo.URL) {
		return invalid("repo.url (required)")
	}
	for _, check := range []struct {
		key string
		ok  bool
	}{
		{"host", optional(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`, c.Host)},
		{"repo.branch", optional(`^[A-Za-z0-9][A-Za-z0-9/_.-]{0,254}$`, c.Repo.Branch)},
		{"repo.path", c.Repo.Path == "" || homePath(c.Repo.Path)},
		{"git.backend", oneOf(c.Git.Backend, "auto", "system", "embedded")},
		{"sync.mode", oneOf(c.Sync.Mode, "link", "copy")},
		{"sync.poll_interval", optional(`^([0-9]+(ms|s|m|h))+$`, c.Sync.PollInterval)},
		{"ui.theme", oneOf(c.UI.Theme, "auto", "dark", "light")},
		{"ui.color", oneOf(c.UI.Color, "auto", "always", "never")},
	} {
		if !check.ok {
			return invalid(check.key)
		}
	}
	seen := map[string]bool{}
	for _, root := range c.Migration.LegacyRoots {
		if !homePath(root) || seen[root] {
			return invalid("migration.legacy_roots")
		}
		seen[root] = true
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	if value == "" {
		return true
	}
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func homePath(value string) bool {
	if !strings.HasPrefix(value, "~/") || len(value) > 4096 || strings.ContainsAny(value, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(value[2:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// WorkingCopy resolves repo.path or the repository's platform data location.
func (c Config) WorkingCopy(env platform.Env) (string, error) {
	if c.Repo.URL == "" {
		return "", invalid("repo.url (configure an existing clone)")
	}
	if c.Repo.Path != "" {
		home, err := env.HomeDir()
		if err != nil {
			return "", err
		}
		return Expand(home, c.Repo.Path), nil
	}
	name, err := vcs.RepoName(c.Repo.URL)
	if err != nil {
		return "", err
	}
	return vcs.WorkingCopyDir(env, name)
}

// Expand resolves a validated home-relative configuration path.
func Expand(home, value string) string {
	return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(value, "~/")))
}
