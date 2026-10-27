package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// ListHost adds the opt-in, names-only directory listing to discovery's host.
type ListHost interface {
	Host
	ReadDir(string) ([]fs.DirEntry, error)
}

// Snapshot is the read-only agents_schema=1 listing. A zero manifest schema
// means no manifest is configured yet. Publication arrives in a later rollout.
type Snapshot struct {
	Host         string            `json:"host"`
	KnownHosts   []string          `json:"known_hosts"`
	Kinds        []string          `json:"kinds"`
	Instances    []Instance        `json:"instances"`
	Candidates   []ListedCandidate `json:"candidates"`
	Warnings     []string          `json:"warnings"`
	AgentsSchema int               `json:"agents_schema"`
	Schema       int               `json:"schema"`
}

// ListedCandidate contains only directory metadata, never an OS error message.
type ListedCandidate struct {
	Kind       string   `json:"kind"`
	Dir        string   `json:"dir"`
	Source     Source   `json:"source"`
	Variable   string   `json:"variable,omitempty"`
	Presence   Presence `json:"presence"`
	ErrorClass string   `json:"error_class,omitempty"`
}

// Instance describes a declared or implicit target instance. It applies to
// Hosts (empty: every host) except ExcludedHosts. In schema 1 they represent
// host replacement sets; in schema 2 Hosts is the instance's own filter and
// ExcludedHosts lists the hosts whose patch sets enabled = false. Enabled is
// this host's value when the instance applies here, else the declared one;
// ActiveHere includes Enabled.
type Instance struct {
	ListedCandidate
	Target        string   `json:"target"`
	Name          string   `json:"name"`
	Hosts         []string `json:"hosts"`
	ExcludedHosts []string `json:"excluded_hosts"`
	Implicit      bool     `json:"implicit"`
	Enabled       bool     `json:"enabled"`
	Settings      bool     `json:"settings"`
	ActiveHere    bool     `json:"active_here"`
}

// List observes defaults, all declarations (including other hosts), and,
// only when probe is true, matching names from one listing of HOME.
func (t Table) List(host ListHost, m *manifest.Manifest, current string, probe bool) (Snapshot, error) {
	result := Snapshot{AgentsSchema: 1, Host: current, KnownHosts: []string{}, Kinds: []string{}, Instances: []Instance{}, Candidates: []ListedCandidate{}, Warnings: []string{}}
	home, err := host.HomeDir()
	if err != nil {
		return result, err
	}
	defaults, err := t.Defaults(host)
	if err != nil {
		return result, err
	}
	if m != nil {
		result.Schema = m.Schema
		result.KnownHosts = append([]string{}, slices.Sorted(maps.Keys(m.Hosts))...)
		if err = t.instances(host, home, m, current, &result); err != nil {
			return result, err
		}
	}
	for _, resolver := range t {
		if resolver.Home != nil || hasKind(result.Instances, resolver.Agent) {
			result.Kinds = append(result.Kinds, resolver.Agent)
		}
	}
	for _, candidate := range defaults {
		result.addCandidate(listed(home, candidate))
	}
	if probe {
		err = t.probe(host, home, &result)
	}
	return result, err
}

func hasKind(instances []Instance, kind string) bool {
	return slices.ContainsFunc(instances, func(i Instance) bool { return i.Kind == kind })
}

func listed(home string, c Candidate) ListedCandidate {
	result := ListedCandidate{Kind: c.Agent, Dir: c.Directory, Source: c.Source, Variable: c.Variable, Presence: c.Presence}
	rel, err := filepath.Rel(home, c.Directory)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		result.Presence = OutsideHome
	}
	if result.Presence == Unreadable {
		result.ErrorClass = "io"
	}
	if result.Presence == Unreadable && errors.Is(c.Err, fs.ErrPermission) {
		result.ErrorClass = "permission"
	}
	return result
}

func (s *Snapshot) addCandidate(c ListedCandidate) {
	for _, instance := range s.Instances {
		if instance.Kind == c.Kind && instance.Dir == c.Dir {
			return
		}
	}
	for _, candidate := range s.Candidates {
		if candidate.Kind == c.Kind && candidate.Dir == c.Dir {
			return
		}
	}
	s.Candidates = append(s.Candidates, c)
}

func (t Table) instances(host Host, home string, m *manifest.Manifest, current string, result *Snapshot) error {
	effective, err := m.ResolveHost(selectedHost{host, current}, current)
	if err != nil {
		return err
	}
	for _, resolver := range t {
		for _, target := range slices.Sorted(maps.Keys(m.Targets)) {
			if m.Targets[target].Agent != resolver.Agent {
				continue
			}
			if err = t.targetInstances(host, home, m, target, effective.Targets[target], current, result); err != nil {
				return err
			}
		}
	}
	return nil
}

// targetInstances lists one target's instances for every host scope.
func (t Table) targetInstances(host Host, home string, m *manifest.Manifest, target string, effective manifest.Target, current string, result *Snapshot) error {
	base := m.Targets[target]
	if m.Schema >= schemaInstanceHosts && len(base.Instances) > 0 {
		return t.filteredInstances(host, home, m, target, effective, current, result)
	}
	excluded := replacementHosts(m, target, result.KnownHosts)
	if err := t.appendInstances(host, home, target, base, nil, excluded, effective, current, result); err != nil {
		return err
	}
	for _, name := range excluded {
		resolved, err := m.ResolveHost(selectedHost{host, name}, name)
		if err != nil {
			return err
		}
		if err = t.appendInstances(host, home, target, resolved.Targets[target], []string{name}, nil, effective, current, result); err != nil {
			return err
		}
	}
	return nil
}

// schemaInstanceHosts is the first manifest schema with instance hosts.
const schemaInstanceHosts = 2

func (t Table) appendInstances(host Host, home, target string, declaration manifest.Target, hosts, excluded []string, effective manifest.Target, current string, result *Snapshot) error {
	instances := declaration.Instances
	if len(instances) == 0 {
		instances = map[string]manifest.Instance{"default": {}}
	}
	for _, name := range slices.Sorted(maps.Keys(instances)) {
		value := instances[name]
		c, err := t.instanceCandidate(host, home, declaration.Agent, value.Dir)
		if err != nil {
			return err
		}
		if c.Dir == "" {
			continue
		}
		implicit := len(declaration.Instances) == 0
		scope := inScope(current, hosts, excluded)
		targetEnabled := declaration.Enabled
		if scope {
			targetEnabled = effective.Enabled
		}
		enabled := on(targetEnabled) && on(value.Enabled)
		active := enabled && scope
		result.Instances = append(result.Instances, Instance{ListedCandidate: c, Target: target, Name: name, Hosts: append([]string{}, hosts...), ExcludedHosts: append([]string{}, excluded...), Implicit: implicit, Enabled: enabled, Settings: declaration.Settings != "" && on(value.Settings), ActiveHere: active})
	}
	return nil
}

// filteredInstances lists a schema-2 target's declared instances with their
// hosts filters and host patches.
func (t Table) filteredInstances(host Host, home string, m *manifest.Manifest, target string, effective manifest.Target, current string, result *Snapshot) error {
	declaration := m.Targets[target]
	for _, name := range slices.Sorted(maps.Keys(declaration.Instances)) {
		value := declaration.Instances[name]
		c, err := t.instanceCandidate(host, home, declaration.Agent, value.Dir)
		if err != nil {
			return err
		}
		excluded := disablingHosts(m, target, name, result.KnownHosts)
		scope := inScope(current, value.Hosts, excluded)
		enabled, settings := on(declaration.Enabled) && on(value.Enabled), value.Settings
		if scope {
			resolved := effective.Instances[name]
			enabled, settings = on(effective.Enabled) && on(resolved.Enabled), resolved.Settings
		}
		result.Instances = append(result.Instances, Instance{ListedCandidate: c, Target: target, Name: name, Hosts: append([]string{}, value.Hosts...), ExcludedHosts: excluded, Enabled: enabled, Settings: declaration.Settings != "" && on(settings), ActiveHere: enabled && scope})
	}
	return nil
}

// disablingHosts lists the hosts whose schema-2 patch disables an instance.
func disablingHosts(m *manifest.Manifest, target, name string, known []string) []string {
	result := []string{}
	for _, h := range known {
		if patch := m.Hosts[h].Targets[target].Instances[name]; patch.Enabled != nil && !*patch.Enabled {
			result = append(result, h)
		}
	}
	return result
}

func inScope(current string, hosts, excluded []string) bool {
	return (len(hosts) == 0 || slices.Contains(hosts, current)) && !slices.Contains(excluded, current)
}

func replacementHosts(m *manifest.Manifest, target string, known []string) []string {
	result := []string{}
	for _, name := range known {
		if m.Hosts[name].Targets[target].Instances != nil {
			result = append(result, name)
		}
	}
	return result
}

func (t Table) instanceCandidate(host Host, home, kind, explicit string) (ListedCandidate, error) {
	dir := filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(explicit, "~/")))
	source, variable := Declared, ""
	if explicit == "" {
		var err error
		dir, err = t.Home(host, kind)
		if err != nil || dir == "" {
			return ListedCandidate{}, err
		}
		source, variable = t.defaultSource(host, kind)
	}
	presence, err := observe(host, dir)
	return listed(home, Candidate{Agent: kind, Directory: dir, Source: source, Variable: variable, Presence: presence, Err: err}), nil
}

func on(value *bool) bool { return value == nil || *value }

func (t Table) defaultSource(host Host, agent string) (Source, string) {
	for _, resolver := range t {
		if resolver.Agent == agent && relocated(host, resolver.Variable) {
			return Environment, resolver.Variable
		}
	}
	return Default, ""
}

type selectedHost struct {
	Host
	name string
}

// LookupEnv makes explicit host selection win over the caller environment.
func (h selectedHost) LookupEnv(key string) (string, bool) {
	if key == "KITBAG_HOST" {
		return h.name, true
	}
	return h.Host.LookupEnv(key)
}

func (t Table) probe(host ListHost, home string, result *Snapshot) error {
	entries, err := host.ReadDir(home)
	if err != nil {
		return fmt.Errorf("probe HOME: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		for _, resolver := range t {
			if !probeName(name, resolver.ProbePrefix) {
				continue
			}
			dir := filepath.Join(home, name)
			presence, observeErr := observe(host, dir)
			result.addCandidate(listed(home, Candidate{Agent: resolver.Agent, Directory: dir, Source: Probe, Presence: presence, Err: observeErr}))
		}
	}
	return nil
}

func probeName(name, prefix string) bool {
	if prefix == "" || !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	return len(suffix) > 1 && strings.ContainsRune("_.-", rune(suffix[0])) && !strings.ContainsAny(suffix, "/\\")
}
