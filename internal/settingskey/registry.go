// Package settingskey defines machine settings and their precedence independently
// of TOML, Git backends and command renderers.
package settingskey

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Key describes one leaf in config.toml. Empty defaults mean unset.
type Key struct {
	Path, Type, Default, Env, Flag, Description, Pattern string
	Allowed                                              []string
	Sensitive                                            bool
}

// Registry returns independent descriptors in display order.
func Registry() []Key {
	return []Key{
		{Path: "schema", Type: "integer", Default: "1", Description: "Configuration schema version.", Allowed: []string{"1"}},
		{Path: "host", Type: "string", Env: "KITBAG_HOST", Flag: "host", Description: "Manifest host selector; default is the machine's short hostname.", Pattern: `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`},
		{Path: "repo.url", Type: "string", Env: "KITBAG_REPO_URL", Flag: "repo", Description: "Configuration repository URL; required in a saved file. Never include credentials.", Pattern: `^[^\s]+$`, Sensitive: true},
		{Path: "repo.branch", Type: "string", Env: "KITBAG_REPO_BRANCH", Flag: "branch", Description: "Branch to follow; unset follows the remote default.", Pattern: `^[A-Za-z0-9][A-Za-z0-9/_.-]{0,254}$`},
		{Path: "repo.path", Type: "string", Env: "KITBAG_REPO_PATH", Description: "Working copy below HOME; unset uses the platform data directory.", Pattern: "home path, at most 4096 bytes"},
		{Path: "git.backend", Type: "string", Default: "auto", Env: "KITBAG_GIT_BACKEND", Flag: "git-backend", Description: "Prefer system Git automatically, or force one backend.", Allowed: []string{"auto", "system", "embedded"}},
		{Path: "git.identity.name", Type: "string", Env: "KITBAG_GIT_IDENTITY_NAME", Flag: "identity-name", Description: "Fallback embedded commit author; name and email must be configured together.", Pattern: `^[^<>\r\n\x00]+$`, Sensitive: true},
		{Path: "git.identity.email", Type: "string", Env: "KITBAG_GIT_IDENTITY_EMAIL", Flag: "identity-email", Description: "Fallback embedded commit email; name and email must be configured together.", Pattern: `^[^<>\s\x00]+@[^<>\s\x00]+$`, Sensitive: true},
		{Path: "sync.poll_interval", Type: "string", Default: "60s", Env: "KITBAG_SYNC_POLL_INTERVAL", Description: "Remote head polling interval; 1s minimum, 24h maximum.", Pattern: `^([0-9]+(ms|s|m|h))+$`},
		{Path: "daemon.debounce", Type: "string", Default: "500ms", Env: "KITBAG_DAEMON_DEBOUNCE", Description: "Watcher coalescing window; 100ms minimum, 1m maximum.", Pattern: `^([0-9]+(ms|s|m|h))+$`},
		{Path: "daemon.update_check", Type: "boolean", Default: "true", Env: "KITBAG_DAEMON_UPDATE_CHECK", Description: "Check release metadata daily; never download or install.", Allowed: []string{"true", "false"}},
		{Path: "daemon.release_url", Type: "string", Default: "https://api.github.com/repos/TomaszJanusz/kitbag/releases/latest", Env: "KITBAG_DAEMON_RELEASE_URL", Description: "Latest release JSON endpoint; restart daemon after changing daemon settings.", Pattern: `^https?://[^\s]+$`},
		{Path: "sync.mode", Type: "string", Default: platform.DefaultMode(), Env: "KITBAG_MODE", Flag: "mode", Description: "Machine materialization default; manifest copy still wins.", Allowed: []string{"link", "copy"}},
		{Path: "ui.notifications", Type: "boolean", Default: "true", Env: "KITBAG_UI_NOTIFICATIONS", Description: "Show notifications after changes (desktop UI is planned).", Allowed: []string{"true", "false"}},
		{Path: "ui.theme", Type: "string", Default: "auto", Env: "KITBAG_UI_THEME", Description: "UI theme (desktop UI is planned).", Allowed: []string{"auto", "dark", "light"}},
		{Path: "ui.color", Type: "string", Default: "auto", Env: "KITBAG_UI_COLOR", Description: "Terminal color preference; NO_COLOR disables color.", Allowed: []string{"auto", "always", "never"}},
		{Path: "migration.legacy_roots", Type: "array", Default: "[]", Env: "KITBAG_MIGRATION_LEGACY_ROOTS", Description: "Unique explicit legacy HOME-relative roots; TOML string array.", Pattern: "unique home paths, each at most 4096 bytes"},
	}
}

// Find rejects unknown setting names.
func Find(path string) (Key, error) {
	for _, key := range Registry() {
		if key.Path == path {
			return key, nil
		}
	}
	return Key{}, fmt.Errorf("unknown setting %q: %w", path, fs.ErrInvalid)
}

type level struct{ value, source, description string }

func (k Key) levels(flag, env, file string) []level {
	return []level{
		{flag, "--" + k.Flag, "flag"},
		{env, k.Env, "KITBAG_* environment"},
		{file, k.Path + " in config.toml", "config.toml"},
		{k.Default, "default", "default"},
	}
}

// Precedence describes the same ordered table used by Choose.
func Precedence() string {
	levels := (Key{}).levels("", "", "")
	descriptions := make([]string, 0, len(levels))
	for _, level := range levels {
		descriptions = append(descriptions, level.description)
	}
	return strings.Join(descriptions, " > ")
}

// Choose is the single precedence table: flag > environment > file > default.
// Empty flag/environment strings are unset, matching existing CLI behavior.
func (k Key) Choose(flag, env, file string) (string, string) {
	for _, level := range k.levels(flag, env, file) {
		if level.value != "" {
			return level.value, level.source
		}
	}
	return "", "default"
}

// Validate checks a textual scalar; arrays are validated by the TOML layer.
func (k Key) Validate(value string) error {
	valid := value != ""
	if len(k.Allowed) > 0 {
		valid = slices.Contains(k.Allowed, value)
	}
	if strings.HasPrefix(k.Pattern, "^") {
		valid = regexp.MustCompile(k.Pattern).MatchString(value)
	}
	if k.Path == "sync.poll_interval" || k.Path == "daemon.debounce" {
		duration, err := time.ParseDuration(value)
		const maxPoll = 24 * time.Hour
		const minDebounce = 100 * time.Millisecond
		minimum, maximum := time.Second, maxPoll
		if k.Path == "daemon.debounce" {
			minimum, maximum = minDebounce, time.Minute
		}
		valid = valid && err == nil && duration >= minimum && duration <= maximum
	}
	if !valid {
		return fmt.Errorf("invalid configuration %s: allowed %s: %w", k.Path, k.Constraint(), fs.ErrInvalid)
	}
	return nil
}

// Constraint describes the allowed values or grammar for error messages and docs.
func (k Key) Constraint() string {
	if k.Path == "sync.poll_interval" {
		return "duration from 1s to 24h (ms, s, m, h)"
	}
	if k.Path == "daemon.debounce" {
		return "duration from 100ms to 1m (ms, s, m, h)"
	}
	if len(k.Allowed) > 0 {
		return strings.Join(k.Allowed, ", ")
	}
	if k.Pattern != "" {
		return k.Pattern
	}
	return k.Type
}

// DefaultDescription includes defaults resolved later by host and Git contracts.
func (k Key) DefaultDescription() string {
	switch k.Path {
	case "host":
		return "machine short hostname"
	case "repo.path":
		return "platform data directory / repos / repository name"
	case "repo.branch":
		return "remote default branch"
	}
	if k.Default == "" {
		return "unset"
	}
	return k.Default
}
