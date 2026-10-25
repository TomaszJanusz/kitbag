// Package xdg resolves the home, kitbag and agent directories shared by the
// unix and fake hosts from an environment lookup function.
//
// Linux and macOS use the same XDG Base Directory layout; docs/platform.md
// records why macOS does not use ~/Library/Application Support.
package xdg

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// app is the directory name below each XDG base directory.
const app = "kitbag"

var (
	errHomeUnset   = errors.New("HOME is not set")
	errNotAbsolute = errors.New("must be an absolute path")
	errRelNotLocal = errors.New("agent home must be a relative path below the home directory")
)

// Lookup reads an environment variable, like os.LookupEnv.
type Lookup func(key string) (string, bool)

// RuntimeDir resolves the private local socket directory without creating it.
func RuntimeDir(lookup Lookup) (string, error) {
	if dir, _ := lookup("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		return filepath.Join(dir, app), nil
	}
	dirs, err := Dirs(lookup)
	if err != nil {
		return "", err
	}
	return filepath.Join(dirs.State, "run"), nil
}

// Home returns the cleaned value of HOME, which must be absolute.
func Home(lookup Lookup) (string, error) {
	home, _ := lookup("HOME")
	switch {
	case home == "":
		return "", errHomeUnset
	case !filepath.IsAbs(home):
		return "", fmt.Errorf("HOME=%q %w: %w", home, errNotAbsolute, fs.ErrInvalid)
	}
	return filepath.Clean(home), nil
}

// Dirs resolves kitbag's directories. Following the XDG specification, a base
// variable that is unset, empty or relative is ignored in favor of its default.
func Dirs(lookup Lookup) (platform.Dirs, error) {
	home, err := Home(lookup)
	if err != nil {
		return platform.Dirs{}, err
	}
	base := func(key string, def ...string) string {
		if dir, _ := lookup(key); filepath.IsAbs(dir) {
			return filepath.Join(dir, app)
		}
		return filepath.Join(home, filepath.Join(def...), app)
	}
	return platform.Dirs{
		Config: base("XDG_CONFIG_HOME", ".config"),
		Data:   base("XDG_DATA_HOME", ".local", "share"),
		State:  base("XDG_STATE_HOME", ".local", "state"),
		Cache:  base("XDG_CACHE_HOME", ".cache"),
	}, nil
}

// AgentHome implements platform.Env.AgentHome. Unlike XDG variables, an agent
// override is never ignored: the agent itself honors it, so a relative value
// is an error rather than a silent fallback to a directory it does not read.
func AgentHome(lookup Lookup, override, rel string) (string, error) {
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("%q: %w: %w", rel, errRelNotLocal, fs.ErrInvalid)
	}
	if override != "" {
		if dir, _ := lookup(override); dir != "" {
			if !filepath.IsAbs(dir) {
				return "", fmt.Errorf("%s=%q %w: %w", override, dir, errNotAbsolute, fs.ErrInvalid)
			}
			return filepath.Clean(dir), nil
		}
	}
	home, err := Home(lookup)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, local), nil
}
