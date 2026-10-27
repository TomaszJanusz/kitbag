// Package manifestupgrade converts a schema-1 kitbag.toml to schema 2 without
// losing anything (ADR 12, "Migration: kitbag manifest upgrade"): it bumps
// schema and min_tool_version, turns each host instance table that replaces a
// target's set into the target-level hosts filter and host patches, and proves
// the result resolves to the same active instances on every host the file
// names and on one it does not. Edits go through internal/core/manifestedit,
// so comments, order and layout stay. It writes nothing.
package manifestupgrade

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// FirstSchema2Release is the first KitBag release that reads manifest schema
// 2. The upgrade writes it as min_tool_version, and every machine must run it
// or later before an upgraded manifest is published.
const FirstSchema2Release = "0.1.4"

// Title is the commit title of the upgrade.
const Title = "chore(kitbag): upgrade manifest to schema 2"

// ErrRefused is a schema-1 layout the upgrade cannot convert losslessly; the
// user edits it by hand. It also matches fs.ErrInvalid (exit 2).
var ErrRefused = errors.New("cannot upgrade the manifest automatically")

// Env is the caller's environment, used only to warn about relocation
// variables when an implicit default directory gets declared.
type Env interface {
	LookupEnv(key string) (string, bool)
}

// Input is everything Prepare needs.
type Input struct {
	Env Env
	// Filename labels diagnostics.
	Filename string
	// HostName is recorded in the Kitbag-Host commit trailer.
	HostName string
	Manifest []byte
	// Kinds supplies each native kind's documented default directory.
	Kinds discover.Table
}

// Plan is a validated upgrade that has not been written. Current means the
// manifest already uses schema 2 and there is nothing to do.
type Plan struct {
	Revision       string              `json:"revision"`
	Title          string              `json:"title"`
	Message        string              `json:"message"`
	MinToolVersion string              `json:"min_tool_version"`
	Conversions    []string            `json:"conversions"`
	Diff           []string            `json:"diff"`
	Warnings       []string            `json:"warnings"`
	Hosts          []string            `json:"checked_hosts"`
	Edits          []manifestedit.Edit `json:"-"`
	After          []byte              `json:"-"`
	SchemaBefore   int                 `json:"schema_before"`
	Current        bool                `json:"already_current"`
}

// Prepare plans the upgrade of in.Manifest.
func Prepare(in Input) (Plan, error) {
	m, err := manifest.Parse(in.Filename, in.Manifest)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Revision: agents.Revision(in.Manifest), SchemaBefore: m.Schema, Conversions: []string{}, Diff: []string{}, Warnings: []string{}, Hosts: []string{}}
	if m.Schema >= manifest.SchemaLatest {
		p.Current, p.MinToolVersion = true, m.MinToolVersion
		return p, nil
	}
	c := &converter{in: in, m: m, plan: &p, defaults: map[string]string{}}
	if err = c.convert(); err != nil {
		return Plan{}, err
	}
	if p.After, err = manifestedit.Apply(in.Filename, in.Manifest, p.Edits...); err != nil {
		if errors.Is(err, manifestedit.ErrLayout) {
			return Plan{}, refused("%v", err)
		}
		return Plan{}, err
	}
	if err = c.verify(); err != nil {
		return Plan{}, err
	}
	p.Diff = agents.Diff(in.Manifest, p.After)
	p.Title, p.Message = Title, message(p.Conversions, in.HostName)
	p.Warnings = append(p.Warnings, readiness(p.MinToolVersion))
	return p, nil
}

type converter struct {
	m        *manifest.Manifest
	plan     *Plan
	defaults map[string]string // target → name of a declared implicit default
	in       Input
}

func (c *converter) convert() error {
	c.plan.MinToolVersion = c.m.MinToolVersion
	if c.m.MinToolVersion == "" || less(c.m.MinToolVersion, FirstSchema2Release) {
		c.plan.MinToolVersion = FirstSchema2Release
	}
	c.edit(manifestedit.Edit{Op: manifestedit.SetSchema, Schema: manifest.SchemaLatest}, "set schema = 2")
	if c.plan.MinToolVersion != c.m.MinToolVersion {
		c.edit(manifestedit.Edit{Op: manifestedit.SetMinToolVersion, Version: c.plan.MinToolVersion}, "set min_tool_version = "+c.plan.MinToolVersion+", the first release that reads schema 2")
	}
	for _, target := range slices.Sorted(maps.Keys(c.m.Targets)) {
		if err := c.target(target); err != nil {
			return err
		}
	}
	return nil
}

func (c *converter) edit(e manifestedit.Edit, conversion string) {
	c.plan.Edits = append(c.plan.Edits, e)
	c.plan.Conversions = append(c.plan.Conversions, conversion)
}

// overriding lists the hosts whose table replaces target's instance set.
func (c *converter) overriding(target string) []string {
	var hosts []string
	for _, host := range slices.Sorted(maps.Keys(c.m.Hosts)) {
		if c.m.Hosts[host].Targets[target].Instances != nil {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

// moved is an instance that only host tables declare.
type moved struct {
	instance manifest.Instance
	hosts    []string
}

func (c *converter) target(target string) error {
	hosts := c.overriding(target)
	if len(hosts) == 0 {
		return nil
	}
	base := maps.Clone(c.m.Targets[target].Instances)
	if len(base) == 0 {
		name, err := c.declareDefault(target, hosts)
		if err != nil {
			return err
		}
		base = map[string]manifest.Instance{name: {Dir: c.defaultDir(target)}}
	}
	only := map[string]*moved{}
	for _, host := range hosts {
		if err := c.hostSet(target, host, base, only); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(only)) {
		mv := only[name]
		c.edit(manifestedit.Edit{Op: manifestedit.MoveHostInstance, Host: mv.hosts[0], Target: target, Name: name, Hosts: mv.hosts},
			fmt.Sprintf("move instance %s of [hosts.%s.targets.%s.instances] to [targets.%s.instances.%s] with hosts = [%s]", name, mv.hosts[0], target, target, name, strings.Join(mv.hosts, ", ")))
	}
	return nil
}

func (c *converter) defaultDir(target string) string {
	for _, r := range c.in.Kinds {
		if r.Agent == c.m.Targets[target].Agent {
			return r.DefaultDir
		}
	}
	return ""
}

// declareDefault declares a target's implicit default before its host
// tables are converted, because in schema 2 they can only patch it.
func (c *converter) declareDefault(target string, hosts []string) (string, error) {
	dir := c.defaultDir(target)
	if dir == "" {
		return "", refused("target %s has no listed instances and agent %s has no documented default directory to declare; list its instances under [targets.%s.instances] by hand first", target, c.m.Targets[target].Agent, target)
	}
	name := ""
	for _, candidate := range []string{"main", "default"} {
		if c.nameFree(target, hosts, candidate, dir) {
			name = candidate
			break
		}
	}
	if name == "" {
		return "", refused("target %s: the names main and default are taken by host instances with other directories; declare the default directory %s under [targets.%s.instances] by hand first", target, dir, target)
	}
	c.defaults[target] = name
	c.edit(manifestedit.Edit{Op: manifestedit.AddInstance, Target: target, Name: name, Dir: dir},
		fmt.Sprintf("declare the implicit default of target %s as instance %s (%s), so host tables can patch it", target, name, dir))
	c.relocated(target, dir)
	return name, nil
}

// relocated warns when this machine relocates a declared default directory.
func (c *converter) relocated(target, dir string) {
	if c.in.Env == nil {
		return
	}
	for _, r := range c.in.Kinds {
		if r.Agent != c.m.Targets[target].Agent || r.Variable == "" {
			continue
		}
		if value, ok := c.in.Env.LookupEnv(r.Variable); ok && value != "" {
			c.plan.Warnings = append(c.plan.Warnings, fmt.Sprintf("%s is set here (%s); the declared default of target %s is %s on every host, so a machine that relocates it must declare its real directory", r.Variable, value, target, dir))
		}
	}
}

func (c *converter) nameFree(target string, hosts []string, name, dir string) bool {
	for _, host := range hosts {
		if instance, ok := c.m.Hosts[host].Targets[target].Instances[name]; ok && instance.Dir != dir {
			return false
		}
	}
	return true
}

// hostSet converts one host's replacement set of target, with base the
// target-level set (including a declared default).
func (c *converter) hostSet(target, host string, base map[string]manifest.Instance, only map[string]*moved) error {
	set := c.m.Hosts[host].Targets[target].Instances
	where := fmt.Sprintf("[hosts.%s.targets.%s.instances", host, target)
	for _, name := range slices.Sorted(maps.Keys(set)) {
		instance := set[name]
		if declared, ok := base[name]; ok {
			if err := c.shared(target, host, name, declared, instance); err != nil {
				return err
			}
			continue
		}
		if err := c.identity(target, host, name, instance, base); err != nil {
			return err
		}
		if mv, ok := only[name]; ok {
			if !reflect.DeepEqual(mv.instance, instance) {
				return refused("%s.%s] and [hosts.%s.targets.%s.instances.%s] define instance %s differently; make them equal or rename one by hand first", where, name, mv.hosts[0], target, name, name)
			}
			mv.hosts = append(mv.hosts, host)
			c.edit(manifestedit.Edit{Op: manifestedit.RemoveHostInstance, Host: host, Target: target, Name: name},
				fmt.Sprintf("drop %s.%s], now covered by the hosts list of the moved instance", where, name))
			continue
		}
		only[name] = &moved{instance: instance, hosts: []string{host}}
	}
	for _, name := range slices.Sorted(maps.Keys(base)) {
		if _, ok := set[name]; !ok {
			c.edit(manifestedit.Edit{Op: manifestedit.AddHostPatch, Host: host, Target: target, Name: name, Enabled: new(bool)},
				fmt.Sprintf("disable instance %s of target %s on host %s with a host patch (its schema-1 set did not list it)", name, target, host))
		}
	}
	return nil
}

// shared converts an instance both the target and a host table declare.
func (c *converter) shared(target, host, name string, declared, instance manifest.Instance) error {
	table := fmt.Sprintf("[hosts.%s.targets.%s.instances.%s]", host, target, name)
	if declared.Dir != instance.Dir || !reflect.DeepEqual(declared.Roots, instance.Roots) || !reflect.DeepEqual(declared.Primitives, instance.Primitives) {
		return refused("%s defines instance %s with a different dir, roots or primitives than [targets.%s.instances.%s]; a schema-2 host table can change only enabled, mode and settings, so rename one of them by hand first", table, name, target, name)
	}
	patch, err := hostPatch(declared, instance)
	if err != nil {
		return refused("%s %v; [targets.%s.instances.%s] sets mode = %q, so align them by hand first", table, err, target, name, declared.Mode)
	}
	patch.Host, patch.Target, patch.Name = host, target, name
	c.edit(manifestedit.Edit{Op: manifestedit.RemoveHostInstance, Host: host, Target: target, Name: name},
		fmt.Sprintf("drop %s, which repeats the target's instance", table))
	if patch.Enabled != nil || patch.Settings != nil || patch.Mode != "" {
		c.edit(patch, fmt.Sprintf("keep its differences on host %s as a host patch of instance %s", host, name))
	}
	return nil
}

// hostPatch expresses how a host's copy of an instance differs from the
// target's as a schema-2 patch.
func hostPatch(declared, instance manifest.Instance) (manifestedit.Edit, error) {
	patch := manifestedit.Edit{Op: manifestedit.AddHostPatch}
	if on(declared.Enabled) != on(instance.Enabled) {
		patch.Enabled = boolean(on(instance.Enabled))
	}
	if on(declared.Settings) != on(instance.Settings) {
		patch.Settings = boolean(on(instance.Settings))
	}
	if declared.Mode != instance.Mode {
		if instance.Mode == "" {
			return patch, errors.New("leaves mode unset, which a host patch cannot express")
		}
		patch.Mode = instance.Mode
	}
	return patch, nil
}

// identity refuses a host-only instance whose directory the target declares
// under another name: two names for one identity.
func (c *converter) identity(target, host, name string, instance manifest.Instance, base map[string]manifest.Instance) error {
	for _, other := range slices.Sorted(maps.Keys(base)) {
		if strings.EqualFold(base[other].Dir, instance.Dir) {
			return refused("[hosts.%s.targets.%s.instances.%s] and [targets.%s.instances.%s] name the same directory %s; use one name for it by hand first", host, target, name, target, other, instance.Dir)
		}
	}
	return nil
}

// verify reparses the result and compares the active instances of every
// target on every host the file names, and on one it does not, with the
// schema-1 resolution.
func (c *converter) verify() error {
	upgraded, err := manifest.Parse(c.in.Filename, c.plan.After)
	if err != nil {
		return fmt.Errorf("upgraded manifest does not parse: %w", err)
	}
	for _, host := range hostsOf(c.m) {
		before, err := c.active(c.m, host)
		if err != nil {
			return err
		}
		after, err := c.active(upgraded, host)
		if err != nil {
			return err
		}
		if !bytes.Equal(before, after) {
			return fmt.Errorf("the upgraded manifest resolves differently on host %s; nothing was written: %w", host, manifestedit.ErrVerify)
		}
		c.plan.Hosts = append(c.plan.Hosts, host)
	}
	return nil
}

// resolvedInstance is what an active instance contributes on a host.
type resolvedInstance struct {
	Roots      map[string]string
	Primitives map[string]manifest.Primitive
	Dir, Mode  string
	Settings   bool
}

type resolvedTarget struct {
	Primitives map[string]manifest.Primitive
	Instances  map[string]resolvedInstance
	Agent      string
	Mode       string
	Settings   string
	Enabled    bool
}

// active renders the targets of m on host as comparable JSON. A schema-1
// target without listed instances counts as its declared default when the
// upgrade declares one.
func (c *converter) active(m *manifest.Manifest, host string) ([]byte, error) {
	resolved, err := m.ResolveHost(hostEnv(host), host)
	if err != nil {
		return nil, err
	}
	targets := map[string]resolvedTarget{}
	for name, t := range resolved.Targets {
		instances := t.Instances
		if len(instances) == 0 && m.Schema < manifest.SchemaLatest && c.defaults[name] != "" {
			instances = map[string]manifest.Instance{c.defaults[name]: {Dir: c.defaultDir(name)}}
		}
		active := map[string]resolvedInstance{}
		for n, i := range instances {
			if on(i.Enabled) {
				active[n] = resolvedInstance{Roots: i.Roots, Primitives: i.Primitives, Dir: i.Dir, Mode: i.Mode, Settings: on(i.Settings)}
			}
		}
		targets[name] = resolvedTarget{Primitives: t.Primitives, Instances: active, Agent: t.Agent, Mode: t.Mode, Settings: t.Settings, Enabled: on(t.Enabled)}
	}
	return json.Marshal(targets)
}

// hostsOf lists every host the manifest names, plus one it does not.
func hostsOf(m *manifest.Manifest) []string {
	named := map[string]bool{}
	for host := range m.Hosts {
		named[host] = true
	}
	for _, file := range m.Files {
		for _, host := range file.Hosts {
			named[host] = true
		}
	}
	undeclared := "kitbag-undeclared-host"
	for named[undeclared] {
		undeclared += "-x"
	}
	return append(slices.Sorted(maps.Keys(named)), undeclared)
}

// hostEnv selects a host for ResolveHost and exposes no other variable.
type hostEnv string

// LookupEnv returns the host for KITBAG_HOST only.
func (h hostEnv) LookupEnv(key string) (string, bool) {
	if key == "KITBAG_HOST" {
		return string(h), true
	}
	return "", false
}

// Hostname returns the host.
func (h hostEnv) Hostname() (string, error) { return string(h), nil }

func message(conversions []string, host string) string {
	lines := make([]string, 0, len(conversions))
	for _, conversion := range conversions {
		lines = append(lines, "- "+conversion)
	}
	return Title + "\n\n" + strings.Join(lines, "\n") + "\n\nKitbag-Host: " + host + "\nKitbag-Op: manifest.upgrade\n"
}

// readiness is the warning every upgrade carries: machines must be updated
// before the upgraded manifest reaches them.
func readiness(version string) string {
	return "every machine that syncs this repository must run kitbag " + version + " or later before this change is published: " +
		"kitbag 0.1.1 to 0.1.3 stop with 'update kitbag' (exit 4) and 0.1.0 with a validation error (exit 2), applying nothing until updated"
}

type usageError struct{ message string }

func (e *usageError) Error() string   { return e.message }
func (e *usageError) Unwrap() []error { return []error{ErrRefused, fs.ErrInvalid} }

func refused(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

func on(value *bool) bool { return value == nil || *value }

func boolean(value bool) *bool { return &value }

// less compares semantic versions through manifest.CheckVersion.
func less(a, b string) bool {
	return (&manifest.Manifest{MinToolVersion: b}).CheckVersion(a) != nil
}
