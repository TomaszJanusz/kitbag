package manifest

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ResolveHost selects KITBAG_HOST, then configured (config.toml's host), then the
// system's short hostname. env can be an internal/platform.Env. It returns an
// independent copy with matching files, expanded instruction template and merged
// host policy. Instance overrides replace whole sets; publish replaces both keys.
// Mode precedence below target level remains the planner's responsibility.
func (m *Manifest) ResolveHost(env Environment, configured string) (*Manifest, error) {
	host, err := hostName(env, configured)
	if err != nil {
		return nil, err
	}
	encoded, err := toml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("copy manifest: %w", err)
	}
	resolved := &Manifest{doc: m.doc}
	if err = toml.Unmarshal(encoded, resolved); err != nil {
		return nil, fmt.Errorf("copy manifest: %w", err)
	}
	resolved.Targets, err = resolved.hostTargets(host)
	if err != nil {
		return nil, err
	}
	override := resolved.Hosts[host]
	if override.Mode != "" {
		resolved.Mode = override.Mode
	}
	if override.Publish != nil {
		resolved.Publish = override.Publish
	}
	resolved.Hosts = nil
	resolved.Files = slices.DeleteFunc(resolved.Files, func(file File) bool {
		return len(file.Hosts) > 0 && !slices.Contains(file.Hosts, host)
	})
	if resolved.Instructions != nil {
		resolved.Instructions.Hosts = strings.ReplaceAll(resolved.Instructions.Hosts, "{host}", host)
	}
	if err = resolved.validateHostAddition(); err != nil {
		return nil, err
	}
	return resolved, nil
}

func hostName(env Environment, configured string) (string, error) {
	name, ok := env.LookupEnv("KITBAG_HOST")
	if !ok || name == "" {
		name = configured
	}
	if name == "" {
		hostname, err := env.Hostname()
		if err != nil {
			return "", fmt.Errorf("detect hostname: %w", err)
		}
		name, _, _ = strings.Cut(hostname, ".")
	}
	if !matches(textRule(hostPattern), name) {
		return "", fmt.Errorf("invalid host name %q", name)
	}
	if err := portableName(name); err != nil {
		return "", err
	}
	return name, nil
}

func (m *Manifest) hostTargets(host string) (map[string]Target, error) {
	targets := maps.Clone(m.Targets)
	for _, name := range slices.Sorted(maps.Keys(m.Hosts[host].Targets)) {
		override := m.Hosts[host].Targets[name]
		target, ok := targets[name]
		key := pointer(pointer("/hosts", host)+"/targets", name)
		if !ok {
			return nil, m.doc.fail(key, "host override names an undeclared target", nil)
		}
		if override.Enabled != nil && (target.Enabled == nil || *target.Enabled) {
			target.Enabled = override.Enabled
		}
		if override.Mode != "" {
			target.Mode = override.Mode
		}
		if override.Instances != nil {
			target.Instances = override.Instances
		}
		targets[name] = target
	}
	return targets, nil
}
