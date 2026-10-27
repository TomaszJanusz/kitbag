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
// host policy; publish replaces both keys. In schema 1 a host's instance table
// replaces the target's whole set. In schema 2 an instance whose Hosts excludes
// the host is kept with Enabled false, so the target still lists instances and
// never falls back to its implicit default, and a host's instance tables patch
// enabled, mode and settings by name. Mode precedence below target level
// remains the planner's responsibility.
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
	if m.Schema >= schemaInstanceHosts {
		for name, target := range targets {
			target.Instances = excludeInstances(target.Instances, host)
			targets[name] = target
		}
	}
	for _, name := range slices.Sorted(maps.Keys(m.Hosts[host].Targets)) {
		target, ok := targets[name]
		if !ok {
			return nil, m.doc.fail(pointer(pointer("/hosts", host)+"/targets", name), "host override names an undeclared target", nil)
		}
		targets[name] = m.overrideTarget(target, m.Hosts[host].Targets[name])
	}
	return targets, nil
}

// overrideTarget applies one host's override of a target.
func (m *Manifest) overrideTarget(target Target, override HostTarget) Target {
	if override.Enabled != nil && (target.Enabled == nil || *target.Enabled) {
		target.Enabled = override.Enabled
	}
	if override.Mode != "" {
		target.Mode = override.Mode
	}
	switch {
	case override.Instances == nil:
	case m.Schema >= schemaInstanceHosts:
		target.Instances = patchInstances(target.Instances, override.Instances)
	default:
		target.Instances = override.Instances
	}
	return target
}

// AppliesTo reports whether the instance's Hosts filter (schema 2) admits
// host; an instance without one applies to every host.
func (i Instance) AppliesTo(host string) bool {
	return len(i.Hosts) == 0 || slices.Contains(i.Hosts, host)
}

// excludeInstances returns a copy of instances in which those that do not
// apply to host are disabled.
func excludeInstances(instances map[string]Instance, host string) map[string]Instance {
	if instances == nil {
		return nil
	}
	result := maps.Clone(instances)
	for name, instance := range result {
		if !instance.AppliesTo(host) {
			instance.Enabled = new(bool)
			result[name] = instance
		}
	}
	return result
}

// patchInstances returns a copy of instances with each patch's set keys
// applied to the instance of the same name. Validation guarantees that every
// patched instance exists and applies to the host.
func patchInstances(instances, patches map[string]Instance) map[string]Instance {
	result := maps.Clone(instances)
	for name, patch := range patches {
		instance := result[name]
		if patch.Enabled != nil {
			instance.Enabled = patch.Enabled
		}
		if patch.Mode != "" {
			instance.Mode = patch.Mode
		}
		if patch.Settings != nil {
			instance.Settings = patch.Settings
		}
		result[name] = instance
	}
	return result
}
