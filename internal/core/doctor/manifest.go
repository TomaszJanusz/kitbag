package doctor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/adapters/instructions"
	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

func (s Service) intent() (*manifest.Manifest, error) {
	m, _, err := s.selection()
	return m, err
}

// selection returns the host-resolved manifest and the host check: which host
// was selected and from where, warning when a host taken from the system
// hostname misses its instructions addition while other hosts have one.
func (s Service) selection() (*manifest.Manifest, Check, error) {
	m, err := manifest.Load(s.Host, filepath.Join(s.Repository, "kitbag.toml"))
	if err != nil {
		return nil, Check{}, err
	}
	host, source, err := config.Resolve(s.Host, config.Values{"host": s.Config.Host}, "host", s.HostFlag)
	if err != nil {
		return nil, Check{}, err
	}
	check := Check{ID: "host", Status: OK, Message: "host " + host + " (" + source + ")"}
	if source == "default" && m.Instructions != nil {
		mismatch, mismatchErr := instructions.HostMismatch(s.Host, s.Repository, m.Instructions.Hosts, host)
		if mismatchErr != nil {
			return nil, Check{}, mismatchErr
		}
		if mismatch != "" {
			check = Check{ID: "host", Status: Warn, Message: mismatch, Fix: "set this machine's host explicitly: kitbag config set host <name>"}
		}
	}
	m, err = m.ResolveHost(hostEnvironment{s.Host, host}, host)
	return m, check, err
}

type hostEnvironment struct {
	platform.Env
	flag string
}

// LookupEnv gives the explicit host flag priority.
func (e hostEnvironment) LookupEnv(key string) (string, bool) {
	if key == "KITBAG_HOST" && e.flag != "" {
		return e.flag, true
	}
	return e.Env.LookupEnv(key)
}

func (s Service) manifestChecks(context.Context) []Check {
	m, host, err := s.selection()
	if err != nil {
		return []Check{result("manifest", "cannot load manifest/host", "configure repo.path and repair kitbag.toml or host selection", err, Fail)}
	}
	checks := []Check{
		result("tool-version", "kitbag "+s.Version+"; min_tool_version "+m.MinToolVersion, "upgrade kitbag to the manifest's min_tool_version", m.CheckVersion(s.Version), Fail), host,
		{ID: "manifest-schema", Status: OK, Message: fmt.Sprintf("schema %d (supported)", m.Schema)},
	}
	dirs, err := s.instances(m)
	if err != nil {
		return append(checks, result("instances", "cannot resolve instance directories", "use absolute CODEX_HOME/CLAUDE_CONFIG_DIR or explicit instance dirs", err, Fail))
	}
	for _, dir := range dirs {
		info, statErr := s.Host.Stat(dir.path)
		if statErr == nil && !info.IsDir() {
			statErr = errors.New("not a directory")
		}
		checks = append(checks, result("instance-"+dir.name, dir.path, "create or correct instance directory "+dir.path, statErr, Warn))
	}
	return checks
}

type instance struct{ name, path, agent string }

func (s Service) instances(m *manifest.Manifest) ([]instance, error) {
	home, err := s.Host.HomeDir()
	if err != nil {
		return nil, err
	}
	var dirs []instance
	for _, name := range slices.Sorted(maps.Keys(m.Targets)) {
		target := m.Targets[name]
		if target.Enabled != nil && !*target.Enabled {
			continue
		}
		if len(target.Instances) == 0 {
			dir, err := s.Homes.Home(s.Host, target.Agent)
			if err != nil {
				return nil, err
			}
			if dir != "" {
				dirs = append(dirs, instance{name + "/default", dir, target.Agent})
			}
		}
		dirs = append(dirs, listedInstances(home, name, target)...)
	}
	return dirs, nil
}

func listedInstances(home, name string, target manifest.Target) []instance {
	var dirs []instance
	for _, key := range slices.Sorted(maps.Keys(target.Instances)) {
		in := target.Instances[key]
		if in.Enabled == nil || *in.Enabled {
			dirs = append(dirs, instance{name + "/" + key, config.Expand(home, in.Dir), target.Agent})
		}
	}
	return dirs
}
