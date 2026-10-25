package doctor

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/config"
)

func (s Service) legacyChecks(context.Context) []Check {
	if len(s.Config.Migration.LegacyRoots) == 0 {
		return []Check{{ID: "legacy", Status: OK, Message: "skipped: migration.legacy_roots is empty"}}
	}
	home, err := s.Host.HomeDir()
	if err != nil {
		return []Check{result("legacy", "legacy inspection unavailable", "repair HOME", err, Warn)}
	}
	roots := []string{filepath.Join(home, ".agents", "skills")}
	for _, root := range s.Config.Migration.LegacyRoots {
		roots = append(roots, config.Expand(home, root))
	}
	paths := []string{filepath.Join(home, ".local/bin/agent-config-sync"), filepath.Join(home, ".agent-browser/config.json")}
	skillDirs := []string{filepath.Join(home, ".agents/skills")}
	m, loadErr := s.intent()
	if loadErr == nil {
		instances, instanceErr := s.instances(m)
		err = errors.Join(err, instanceErr)
		for _, in := range instances {
			paths = append(paths, filepath.Join(in.path, "AGENTS.md"), filepath.Join(in.path, "CLAUDE.md"))
			skillDirs = append(skillDirs, filepath.Join(in.path, "skills"))
		}
	}
	paths = append(paths, skillDirs...)
	for _, dir := range skillDirs {
		entries, readErr := s.Host.ReadDir(dir)
		err = errors.Join(err, optionalError(readErr))
		for _, entry := range entries {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return []Check{s.legacyLinks(paths, roots, err), s.legacyMarkers(paths), s.legacyUnits(home)}
}

func optionalError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s Service) legacyLinks(paths, roots []string, err error) Check {
	var found []string
	for _, path := range paths {
		managed, linkErr := s.Host.IsManagedLink(path, roots...)
		err = errors.Join(err, optionalError(linkErr))
		if managed {
			found = append(found, path)
		}
	}
	if len(found) > 0 {
		err = errors.Join(err, errors.New(strings.Join(found, ", ")))
	}
	return result("legacy-links", "legacy links", "review old links and migrate/adopt them explicitly before syncing", err, Warn)
}

func (s Service) legacyMarkers(paths []string) Check {
	var err error
	for _, path := range paths {
		if filepath.Base(path) != "AGENTS.md" && filepath.Base(path) != "CLAUDE.md" {
			continue
		}
		data, readErr := s.Host.ReadFile(path)
		err = errors.Join(err, optionalError(readErr))
		if strings.Contains(string(data), "agent-config-managed") || strings.Contains(string(data), "Managed by agent-config-sync") {
			err = errors.Join(err, errors.New(path))
		}
	}
	return result("legacy-markers", "legacy instruction markers (agent settings are never read)", "review legacy managed instructions and migrate them explicitly", err, Warn)
}

func (s Service) legacyUnits(home string) Check {
	xdg, _ := s.Host.LookupEnv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(xdg) {
		xdg = filepath.Join(home, ".config")
	}
	var err error
	for _, dir := range []string{filepath.Join(xdg, "systemd/user"), filepath.Join(home, "Library/LaunchAgents")} {
		entries, readErr := s.Host.ReadDir(dir)
		err = errors.Join(err, optionalError(readErr))
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "agent-config") {
				err = errors.Join(err, errors.New(filepath.Join(dir, entry.Name())))
			}
		}
	}
	return result("legacy-units", "legacy timer/service definitions (activation not queried)", "review and disable old agent-config-sync timers/services manually", err, Warn)
}
