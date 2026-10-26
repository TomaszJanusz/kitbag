package apiclient

import (
	"context"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/ui/model"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// settingsContext enriches either transport's result from the same local host.
// Missing files or unavailable Git never prevent editing the configuration.
func (c Client) settingsContext(ctx context.Context, values []model.SettingValue) {
	if c.Host == nil {
		return
	}
	var cfg config.Config
	cfg.Repo.URL, cfg.Repo.Path = settingValue(values, "repo.url"), settingValue(values, "repo.path")
	repository, err := cfg.WorkingCopy(c.Host)
	var hosts []string
	var id vcs.Identity
	if err == nil {
		hosts, err = instructions.HostAdditions(c.Host, repository, "hosts/{host}.md")
		if err != nil {
			hosts = nil
		}
		id = c.gitIdentity(ctx, repository, settingValue(values, "git.backend"))
	}
	legacy := c.legacyDetected()
	for i := range values {
		switch values[i].Key {
		case "host":
			values[i].Hosts = hosts
		case "git.identity.name":
			values[i].GitValue = id.Name
		case "git.identity.email":
			values[i].GitValue = id.Email
		case "migration.legacy_roots":
			values[i].LegacyDetected = legacy
		}
	}
}

func settingValue(values []model.SettingValue, key string) string {
	for _, value := range values {
		if value.Key == key {
			return value.Value
		}
	}
	return ""
}

func (c Client) gitIdentity(ctx context.Context, repository, kind string) vcs.Identity {
	if c.OpenGit == nil {
		return vcs.Identity{}
	}
	backend, err := c.OpenGit(ctx, kind)
	if err != nil {
		return vcs.Identity{}
	}
	reader, ok := backend.(vcs.IdentityReader)
	if !ok {
		return vcs.Identity{}
	}
	id, err := reader.ReadIdentity(ctx, repository)
	if err != nil {
		return vcs.Identity{}
	}
	return id
}

// Detect only conventional legacy locations, without scanning agent contents or
// inventing trusted roots. The user still configures explicit roots in the CLI.
func (c Client) legacyDetected() bool {
	home, err := c.Host.HomeDir()
	if err != nil {
		return false
	}
	for _, rel := range []string{"src/agent-config", ".local/bin/agent-config-sync"} {
		if _, err := c.Host.Lstat(filepath.Join(home, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}
