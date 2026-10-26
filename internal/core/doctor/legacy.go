package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func (s Service) legacyChecks(ctx context.Context) []Check {
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
	var claudeSettings []instance
	m, loadErr := s.intent()
	if loadErr == nil {
		instances, instanceErr := s.instances(m)
		err = errors.Join(err, instanceErr)
		for _, in := range instances {
			if in.agent == "claude" {
				claudeSettings = append(claudeSettings, in)
			}
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
	owned := s.owned()
	paths = slices.DeleteFunc(paths, func(path string) bool { return owned[path] })
	return []Check{s.legacyLinks(paths, roots, err), s.legacyMarkers(paths), s.legacySettingsMarkers(claudeSettings), s.legacyUnits(ctx, home)}
}

// owned returns the destinations KitBag owns. Adopted links into a legacy root
// and adopted files keeping agent-config-sync's marker are no longer legacy;
// their drift is plan and status output. Without readable state (busy lock,
// invalid file) nothing is excluded.
func (s Service) owned() map[string]bool {
	owned := map[string]bool{}
	if s.Store == nil {
		return owned
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		db, err := session.Load()
		for _, t := range db.Targets {
			owned[t.Path] = true
		}
		return err
	})
	if err != nil {
		return map[string]bool{}
	}
	return owned
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
	return result("legacy-markers", "legacy instruction markers", "review legacy managed instructions and migrate them explicitly", err, Warn)
}

// legacyUnits finds definitions whose names contain agent-config. Those of the
// host's own manager (systemd user units on Linux, LaunchAgents on macOS) are
// inspected read-only: one that is neither loaded nor enabled no longer runs
// and is not reported. Other definitions, or a failed inspection, are reported
// by presence, as activation cannot be known.
func (s Service) legacyUnits(ctx context.Context, home string) Check {
	xdg, _ := s.Host.LookupEnv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(xdg) {
		xdg = filepath.Join(home, ".config")
	}
	manager := ""
	if layout, err := s.Host.ServiceLayout(); err == nil {
		manager = layout.Manager
	}
	var err error
	disabled := 0
	for _, dir := range []unitDir{
		{filepath.Join(xdg, "systemd/user"), "systemd", ""},
		{filepath.Join(home, "Library/LaunchAgents"), "launchd", ".plist"},
	} {
		entries, readErr := s.Host.ReadDir(dir.path)
		err = errors.Join(err, optionalError(readErr))
		for _, entry := range entries {
			if !strings.Contains(entry.Name(), "agent-config") {
				continue
			}
			finding := s.unitFinding(ctx, dir, entry.Name(), manager)
			if finding == nil {
				disabled++
			}
			err = errors.Join(err, finding)
		}
	}
	message := "legacy timer/service definitions"
	if disabled > 0 {
		message += fmt.Sprintf(" (%d disabled and not loaded, ignored)", disabled)
	}
	return result("legacy-units", message, "disable and unload old agent-config-sync timers/services (systemctl --user disable --now <unit>; launchctl bootout and launchctl disable gui/<uid>/<label>)", err, Warn)
}

// unitDir is a unit definition directory of one manager; a definition's name
// is its filename without suffix.
type unitDir struct{ path, manager, suffix string }

// unitFinding returns nil for a native unit that is neither loaded nor
// enabled, and otherwise what is reported for the definition.
func (s Service) unitFinding(ctx context.Context, dir unitDir, filename, manager string) error {
	path := filepath.Join(dir.path, filename)
	name, native := strings.CutSuffix(filename, dir.suffix)
	if !native || dir.manager != manager {
		return errors.New(path + " (activation not queried)")
	}
	bounded, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	unit, err := s.Host.UnitInspect(bounded, name)
	switch {
	case err != nil:
		return fmt.Errorf("%s (state unknown: %w)", path, err)
	case unit.Loaded || unit.Enabled:
		return fmt.Errorf("%s (loaded=%t enabled=%t)", path, unit.Loaded, unit.Enabled)
	}
	return nil
}

// settingsScanLimit bounds the bytes inspected in one settings file.
const settingsScanLimit = 4 << 20

const (
	settingsMarker       = "# agent-config-managed"
	settingsMarkerFix    = "run kitbag adopt, then kitbag sync; markers are migrated transparently, see docs/adopt.md"
	settingsInspectedFix = "make settings.json readable and smaller than 4 MiB, or inspect it manually"
)

// legacySettingsMarkers reports only the fact and count of legacy hook markers
// in each Claude instance's settings.json. It performs a bounded byte search:
// no parsing, no other field is interpreted, and no content is ever returned.
func (s Service) legacySettingsMarkers(instances []instance) Check {
	var found, skipped []error
	for _, in := range instances {
		n, err := s.countSettingsMarkers(in)
		switch {
		case err != nil:
			skipped = append(skipped, err)
		case n > 0:
			found = append(found, fmt.Errorf("%s: %s: %d legacy hook marker occurrence(s)", in.name, filepath.Join(in.path, "settings.json"), n))
		}
	}
	const message = "legacy hook markers in Claude settings.json (bounded byte search; content never shown)"
	if len(found) == 0 && len(skipped) == 0 {
		return Check{ID: "legacy-settings-markers", Status: OK, Message: message}
	}
	var fixes []string
	if len(found) > 0 {
		fixes = append(fixes, settingsMarkerFix)
	}
	if len(skipped) > 0 {
		fixes = append(fixes, settingsInspectedFix)
	}
	return Check{ID: "legacy-settings-markers", Status: Warn, Message: message + ": " + errors.Join(append(found, skipped...)...).Error(), Fix: strings.Join(fixes, "; ")}
}

// countSettingsMarkers returns the marker count; a missing file counts zero.
// Files over the limit are never read.
func (s Service) countSettingsMarkers(in instance) (int, error) {
	path := filepath.Join(in.path, "settings.json")
	info, err := s.Host.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err == nil && !info.IsDir() && info.Size() > settingsScanLimit {
		return 0, fmt.Errorf("%s: %s: too large to inspect", in.name, path)
	}
	var data []byte
	if err == nil {
		data, err = s.Host.ReadFile(path)
	}
	if err != nil {
		return 0, fmt.Errorf("%s: %s: cannot inspect settings (%w)", in.name, path, err)
	}
	if len(data) > settingsScanLimit {
		return 0, fmt.Errorf("%s: %s: too large to inspect", in.name, path)
	}
	return bytes.Count(data, []byte(settingsMarker)), nil
}
