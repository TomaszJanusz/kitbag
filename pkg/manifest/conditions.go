package manifest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
)

func (d *document) conditional(table map[string]any, key string) error {
	switch {
	case strings.HasPrefix(key, "/targets/") && strings.Count(key, "/") == 2:
		if table["agent"] == "generic" {
			return d.genericTarget(table, key)
		}
	case strings.HasPrefix(key, "/mcp/servers/") && strings.Count(key, "/") == 3:
		return d.server(table, key)
	}
	return nil
}

func (d *document) genericTarget(table map[string]any, key string) error {
	if _, ok := table["settings"]; ok {
		return d.fail(pointer(key, "settings"), "generic does not support settings", nil)
	}
	instances, ok := table["instances"].(map[string]any)
	if !ok {
		return d.fail(key, "missing property 'instances' for generic", nil)
	}
	for _, name := range slices.Sorted(maps.Keys(instances)) {
		instance, valid := instances[name].(map[string]any)
		if !valid {
			continue
		} // The structural validator diagnoses types first.
		if _, exists := instance["roots"]; !exists {
			return d.fail(pointer(pointer(key, "instances"), name), "missing property 'roots'", nil)
		}
	}
	return nil
}

func (d *document) server(table map[string]any, key string) error {
	_, command := table["command"]
	_, url := table["url"]
	if command == url {
		return d.fail(key, "oneOf: exactly one of command and url is required", nil)
	}
	forbidden := []string{"command", "args", "env"}
	allowed := []any{"http", "sse"}
	if command {
		forbidden = []string{"url", "headers"}
		allowed = []any{"stdio"}
	}
	if transport, ok := table["transport"]; ok && !slices.Contains(allowed, transport) {
		return d.fail(pointer(key, "transport"), "oneOf: transport does not match server type", nil)
	}
	for _, name := range forbidden {
		if _, ok := table[name]; ok {
			return d.fail(pointer(key, name), "oneOf: property forbidden for this server type", nil)
		}
	}
	return nil
}

func portableName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid path segment %q", name)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("name %q ends in a dot or space", name)
	}
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return fmt.Errorf("name %q contains a forbidden character", name)
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	if windowsDevice(base) {
		return fmt.Errorf("name %q is reserved on Windows", name)
	}
	return nil
}

func windowsDevice(base string) bool {
	if slices.Contains([]string{"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$"}, base) {
		return true
	}
	for _, prefix := range []string{"COM", "LPT"} {
		rest, ok := strings.CutPrefix(base, prefix)
		if ok && len([]rune(rest)) == 1 && strings.ContainsAny(rest, "123456789¹²³") {
			return true
		}
	}
	return false
}

func (m *Manifest) validateRelations() error {
	if err := m.validateFileDestinations(); err != nil {
		return err
	}
	if err := m.validateImports(); err != nil {
		return err
	}
	if err := m.validateInstances(m.Targets, "/targets"); err != nil {
		return err
	}
	if err := m.validatePatches(); err != nil {
		return err
	}
	for _, host := range slices.Sorted(maps.Keys(m.Hosts)) {
		targets, err := m.hostTargets(host)
		if err != nil {
			return err
		}
		if err = m.validateInstances(targets, pointer("/hosts", host)+"/targets"); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) validateImports() error {
	if m.Skills == nil {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(m.Skills.Imports)) {
		imp := m.Skills.Imports[name]
		if imp.Dir != "" && !slices.Contains(m.Skills.Dirs, imp.Dir) {
			return m.doc.fail(pointer("/skills/imports", name)+"/dir", "import dir must be one of skills.dirs", nil)
		}
	}
	return nil
}

// validatePatches requires every schema-2 host instance patch to name an
// instance its target declares and whose hosts filter admits that host.
func (m *Manifest) validatePatches() error {
	if m.Schema < schemaInstanceHosts {
		return nil
	}
	for _, host := range slices.Sorted(maps.Keys(m.Hosts)) {
		for _, name := range slices.Sorted(maps.Keys(m.Hosts[host].Targets)) {
			target, ok := m.Targets[name]
			if !ok {
				continue // hostTargets reports the undeclared target.
			}
			for _, instanceName := range slices.Sorted(maps.Keys(m.Hosts[host].Targets[name].Instances)) {
				key := pointer(pointer(pointer("/hosts", host)+"/targets", name)+"/instances", instanceName)
				instance, ok := target.Instances[instanceName]
				switch {
				case !ok:
					return m.doc.fail(key, "host instance patch names an instance that targets."+name+" does not declare; declare it there with its dir and limit it to hosts with its 'hosts' key", nil)
				case !instance.AppliesTo(host):
					return m.doc.fail(key, "host instance patch for a host that the instance's hosts list ("+strings.Join(instance.Hosts, ", ")+") excludes; add "+host+" to that list or remove this patch", nil)
				}
			}
		}
	}
	return nil
}

func (m *Manifest) validateInstances(targets map[string]Target, prefix string) error {
	seen := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(targets)) {
		target := targets[name]
		for _, instanceName := range slices.Sorted(maps.Keys(target.Instances)) {
			instance := target.Instances[instanceName]
			key := pointer(pointer(prefix, name)+"/instances", instanceName)
			if target.Agent == "generic" && len(instance.Roots) == 0 {
				return m.doc.fail(key, "missing property 'roots' for generic", nil)
			}
			identity := target.Agent + ":" + foldName(instance.Dir)
			if previous, ok := seen[identity]; ok {
				return m.doc.fail(key, "instance directory collides with "+previous, nil)
			}
			seen[identity] = key
		}
	}
	return nil
}

// foldName canonicalizes Unicode simple case folding, including sigma variants,
// the Kelvin sign and long s, rather than assuming lowercase alone is enough.
func foldName(name string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		smallest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			smallest = min(smallest, next)
		}
		return smallest
	}, name))
}

func (m *Manifest) validateFileDestinations() error {
	for i, file := range m.Files {
		for _, previous := range m.Files[:i] {
			if foldName(file.Dest) != foldName(previous.Dest) || file.Src == previous.Src {
				continue
			}
			if hostsOverlap(file.Hosts, previous.Hosts) {
				return m.doc.fail(fmt.Sprintf("/files/%d/dest", i), "file destination has conflicting sources: "+file.Dest, nil)
			}
		}
	}
	return nil
}

func hostsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, host := range a {
		if slices.Contains(b, host) {
			return true
		}
	}
	return false
}
