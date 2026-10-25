package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	directoryPerm fs.FileMode = 0o700
	filePerm      fs.FileMode = 0o644
	privatePerm   fs.FileMode = 0o600
)

// Service owns install policy; supervisor and filesystem operations are injected.
type Service struct {
	Host   platform.Host
	Config string
}

// Report combines installed definitions and the supervisor's observation.
type Report struct {
	Manager string `json:"manager"`
	Mode    string `json:"mode"`
	platform.ServiceStatus
	Installed     bool `json:"installed"`
	Stale         bool `json:"stale"`
	ServiceSchema int  `json:"service_schema"`
}

// Current resolves fresh, allow-listed input from the host.
func (s Service) Current(mode string, interval time.Duration) (Options, error) {
	layout, err := s.Host.ServiceLayout()
	if err != nil {
		return Options{}, err
	}
	dirs, err := s.Host.Dirs()
	if err != nil {
		return Options{}, err
	}
	config := s.Config
	if config == "" {
		config = filepath.Join(dirs.Config, "config.toml")
	}
	options := Options{Binary: layout.Binary, Config: config, Mode: mode, Interval: interval, LogDir: dirs.LogDir, Environment: map[string]string{}}
	for _, key := range environmentKeys() {
		if value, ok := s.Host.LookupEnv(key); ok {
			options.Environment[key] = value
		}
	}
	return options, options.validate()
}

func (s Service) metadata() (string, error) {
	dirs, err := s.Host.Dirs()
	return filepath.Join(dirs.State, "service.json"), err
}

func (s Service) saved() (Options, error) {
	path, err := s.metadata()
	if err != nil {
		return Options{}, err
	}
	data, err := s.Host.ReadFile(path)
	if err != nil {
		return Options{}, err
	}
	var options Options
	err = json.Unmarshal(data, &options)
	return options, err
}

// Install is idempotent; a changed definition is stopped before replacement.
// noStart enables future starts without starting the supervisor job now.
func (s Service) Install(ctx context.Context, options Options, noStart bool) (changed bool, err error) {
	layout, err := s.Host.ServiceLayout()
	if err != nil {
		return false, err
	}
	files, err := Generate(layout.Manager, options)
	if err != nil {
		return false, err
	}
	changed, err = s.different(layout, files)
	if err != nil {
		return false, err
	}
	_, savedErr := s.saved()
	changed = changed || savedErr != nil
	if changed {
		if err = s.stopInstalled(ctx); err != nil {
			return false, err
		}
		if err = s.write(layout, files, options); err != nil {
			return false, err
		}
	}
	return changed, s.activate(ctx, options.Mode == "timer", noStart)
}

func (s Service) activate(ctx context.Context, timer, noStart bool) error {
	if err := s.Host.ServiceControl(ctx, "reload", false); err != nil {
		return err
	}
	if err := s.Host.ServiceControl(ctx, "enable", timer); err != nil {
		return err
	}
	if noStart {
		return nil
	}
	status, err := s.Host.ServiceInspect(ctx, timer)
	if err != nil || status.Active {
		return err
	}
	return s.Host.ServiceControl(ctx, "start", timer)
}

func (s Service) write(layout platform.ServiceLayout, files map[string]string, options Options) error {
	for _, dir := range []string{layout.Directory, options.LogDir} {
		if err := s.Host.MkdirAll(dir, directoryPerm); err != nil {
			return err
		}
	}
	if err := s.ensureLogs(layout.Manager, options.LogDir); err != nil {
		return err
	}
	if err := s.removeObsolete(layout, files); err != nil {
		return err
	}
	for _, name := range sortedKeys(files) {
		if err := s.Host.WriteAtomic(filepath.Join(layout.Directory, name), []byte(files[name]), filePerm); err != nil {
			return err
		}
	}
	path, err := s.metadata()
	if err != nil {
		return err
	}
	if err = s.Host.MkdirAll(filepath.Dir(path), directoryPerm); err != nil {
		return err
	}
	data, err := json.Marshal(options)
	if err != nil {
		return err
	}
	return s.Host.WriteAtomic(path, append(data, '\n'), privatePerm)
}

func (s Service) different(layout platform.ServiceLayout, files map[string]string) (bool, error) {
	changed := false
	for _, name := range unitNames(layout.Manager) {
		path := filepath.Join(layout.Directory, name)
		data, err := s.Host.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		want, exists := files[name]
		changed = changed || exists && (err != nil || !bytes.Equal(data, []byte(want))) || !exists && err == nil
	}
	return changed, nil
}

func unitNames(manager string) []string {
	if manager == "launchd" {
		return []string{platform.ServiceLabel + ".plist"}
	}
	return []string{platform.ServiceLabel + ".service", platform.ServiceLabel + ".timer"}
}

func (s Service) removeObsolete(layout platform.ServiceLayout, keep map[string]string) error {
	for _, name := range unitNames(layout.Manager) {
		if _, ok := keep[name]; ok {
			continue
		}
		if err := s.Host.Remove(filepath.Join(layout.Directory, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s Service) stopInstalled(ctx context.Context) error {
	// Stop both units when switching modes, including an in-flight timer sync.
	for _, timer := range []bool{true, false} {
		status, err := s.Host.ServiceInspect(ctx, timer)
		if err != nil {
			return err
		}
		if !status.Loaded {
			continue
		}
		if err = s.Host.ServiceControl(ctx, "stop", timer); err != nil {
			return err
		}
		if status.Enabled {
			if err = s.Host.ServiceControl(ctx, "disable", timer); err != nil {
				return err
			}
		}
	}
	return nil
}

// Uninstall removes only owned service files/metadata; configuration, sync state
// and working copies remain available for manual synchronization.
func (s Service) Uninstall(ctx context.Context) error {
	layout, err := s.Host.ServiceLayout()
	if err != nil {
		return err
	}
	if err = s.stopInstalled(ctx); err != nil {
		return err
	}
	if err = s.removeObsolete(layout, nil); err != nil {
		return err
	}
	path, err := s.metadata()
	if err != nil {
		return err
	}
	if err = s.Host.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.Host.ServiceControl(ctx, "reload", false)
}

// Status compares saved mode/interval with freshly generated definitions. A
// missing metadata file with existing units is stale and can be repaired by install.
func (s Service) Status(ctx context.Context) (Report, error) {
	layout, err := s.Host.ServiceLayout()
	result := Report{ServiceSchema: 1, Manager: layout.Manager}
	if err != nil {
		return result, err
	}
	for _, name := range unitNames(layout.Manager) {
		_, statErr := s.Host.Lstat(filepath.Join(layout.Directory, name))
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return result, statErr
		}
		result.Installed = result.Installed || statErr == nil
	}
	if !result.Installed {
		result.Installed, err = s.hasMetadata()
		if err != nil {
			return result, err
		}
	}
	if !result.Installed {
		return result, nil
	}
	return s.installedStatus(ctx, result, layout)
}

func (s Service) installedStatus(ctx context.Context, result Report, layout platform.ServiceLayout) (Report, error) {
	saved, err := s.saved()
	if err != nil {
		result.Stale = true
		return result, errUnlessMissing(err)
	}
	current, err := s.Current(saved.Mode, saved.Interval)
	if err != nil {
		return result, err
	}
	files, err := Generate(layout.Manager, current)
	if err != nil {
		return result, err
	}
	result.Mode = saved.Mode
	result.Stale, err = s.different(layout, files)
	if err != nil {
		return result, err
	}
	result.ServiceStatus, err = s.Host.ServiceInspect(ctx, result.Mode == "timer")
	return result, err
}

func errUnlessMissing(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s Service) ensureLogs(manager, dir string) error {
	if manager != "launchd" {
		return nil
	}
	for _, name := range []string{"service.stdout.log", "service.stderr.log"} {
		path := filepath.Join(dir, name)
		_, err := s.Host.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			err = s.Host.WriteAtomic(path, nil, privatePerm)
		}
		if err != nil {
			return err
		}
		if err = s.Host.Chmod(path, privatePerm); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) hasMetadata() (bool, error) {
	path, err := s.metadata()
	if err != nil {
		return false, err
	}
	_, err = s.Host.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
