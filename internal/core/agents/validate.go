package agents

import (
	"errors"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Names of a declared implicit default; the alternate is used when the new
// instance itself is called main.
const (
	defaultName   = "main"
	alternateName = "default"
)

func (p *planner) add() error {
	c := p.in.Change
	resolver, err := p.kind(c.Kind)
	if err != nil {
		return err
	}
	target, created, err := p.addTarget(c.Kind, c.Target)
	if err != nil {
		return err
	}
	c.Target = target
	if err = manifest.CheckName(c.Name); err != nil {
		return invalid("instance name: %v", err)
	}
	if _, taken := p.m.Targets[target].Instances[c.Name]; taken {
		return invalid("target %s already has an instance named %s", target, c.Name)
	}
	if c.Dir, err = p.dir(c.Dir); err != nil {
		return err
	}
	if err = p.unique(c.Kind, c.Dir, target, ""); err != nil {
		return err
	}
	if err = p.settings(target, c.Settings, created); err != nil {
		return err
	}
	if created {
		p.plan.Edits = append(p.plan.Edits, manifestedit.Edit{Op: manifestedit.AddTarget, Target: target, Agent: c.Kind})
	} else if err = p.keepDefault(resolver, target, c); err != nil {
		return err
	}
	p.plan.Edits = append(p.plan.Edits, manifestedit.Edit{Op: manifestedit.AddInstance, Target: target, Name: c.Name, Dir: c.Dir, Settings: c.Settings})
	p.plan.Change = c
	return p.observe(c.Dir)
}

// kind returns the resolver of a native kind; generic needs explicit roots.
func (p *planner) kind(kind string) (discover.Resolver, error) {
	var known []string
	for _, r := range p.in.Kinds {
		if r.Agent == kind && r.Home != nil {
			return r, nil
		}
		if r.Home != nil {
			known = append(known, r.Agent)
		}
	}
	if kind == "generic" {
		return discover.Resolver{}, unsupported("generic instances need explicit roots; edit kitbag.toml by hand")
	}
	return discover.Resolver{}, invalid("unknown agent kind %q (known: %s)", kind, strings.Join(known, ", "))
}

// addTarget selects the target that receives an added instance, or the name
// of a new target for a kind without one.
func (p *planner) addTarget(kind, requested string) (string, bool, error) {
	if requested != "" {
		t, ok := p.m.Targets[requested]
		switch {
		case !ok:
			return requested, true, p.checkNewTarget(requested)
		case t.Agent != kind:
			return "", false, invalid("target %s uses agent %s, not %s", requested, t.Agent, kind)
		}
		return requested, false, nil
	}
	targets := p.targetsOf(kind)
	switch len(targets) {
	case 0:
		return kind, true, p.checkNewTarget(kind)
	case 1:
		return targets[0], false, nil
	}
	return "", false, invalid("targets %s all use agent %s; choose one with --target", strings.Join(targets, ", "), kind)
}

func (p *planner) checkNewTarget(name string) error {
	if err := manifest.CheckName(name); err != nil {
		return invalid("target name: %v", err)
	}
	if _, ok := p.m.Targets[name]; ok {
		return invalid("target %s already exists", name)
	}
	return nil
}

func (p *planner) targetsOf(kind string) []string {
	var result []string
	for _, name := range slices.Sorted(maps.Keys(p.m.Targets)) {
		if p.m.Targets[name].Agent == kind {
			result = append(result, name)
		}
	}
	return result
}

// keepDefault declares a target's implicit default before its first listed
// instance, because listing any instance replaces the implicit default.
func (p *planner) keepDefault(r discover.Resolver, target string, c Change) error {
	if len(p.m.Targets[target].Instances) > 0 || r.DefaultDir == "" || strings.EqualFold(r.DefaultDir, c.Dir) {
		return nil
	}
	name := defaultName
	if c.Name == name {
		name = alternateName
	}
	if p.unique(r.Agent, r.DefaultDir, target, "") != nil {
		// Another target already manages the default directory.
		return nil
	}
	p.plan.Edits = append(p.plan.Edits, manifestedit.Edit{Op: manifestedit.AddInstance, Target: target, Name: name, Dir: r.DefaultDir})
	p.warn("the default directory %s is declared as instance %s so it stays managed", r.DefaultDir, name)
	if relocated, _ := p.in.Host.LookupEnv(r.Variable); r.Variable != "" && relocated != "" {
		p.warn("%s is set here (%s); the declared default is %s on every host", r.Variable, relocated, r.DefaultDir)
	}
	return nil
}

func (p *planner) edit() error {
	c, err := p.existing()
	if err != nil {
		return err
	}
	current := p.m.Targets[c.Target].Instances[c.Name]
	changed := false
	if p.in.Change.Dir != "" {
		if c.Dir, err = p.dir(p.in.Change.Dir); err != nil {
			return err
		}
		if err = p.unique(c.Kind, c.Dir, c.Target, c.Name); err != nil {
			return err
		}
		changed = c.Dir != current.Dir
	}
	settings := p.in.Change.Settings
	if settings != nil {
		if err = p.settings(c.Target, settings, false); err != nil {
			return err
		}
		changed = changed || on(current.Settings) != *settings
	}
	if !changed {
		return invalid("instance %s/%s already has this directory and settings; nothing to change", c.Target, c.Name)
	}
	edit := manifestedit.Edit{Op: manifestedit.EditInstance, Target: c.Target, Name: c.Name, Settings: settings}
	if c.Dir != current.Dir {
		edit.Dir = c.Dir
		p.plan.Dropped = current.Dir
	}
	c.Settings = settings
	p.plan.Edits, p.plan.Change = []manifestedit.Edit{edit}, c
	return p.observe(c.Dir)
}

func (p *planner) remove() error {
	c, err := p.existing()
	if err != nil {
		return err
	}
	if len(p.m.Targets[c.Target].Instances) == 1 {
		return invalid("%s/%s is the last instance of target %s; without one the target manages its default directory instead, so disable or remove the target in kitbag.toml by hand", c.Target, c.Name, c.Target)
	}
	p.plan.Edits = []manifestedit.Edit{{Op: manifestedit.RemoveInstance, Target: c.Target, Name: c.Name}}
	p.plan.Change, p.plan.Dropped = c, c.Dir
	return nil
}

// existing resolves an edited or removed instance from Target and Name.
func (p *planner) existing() (Change, error) {
	c := p.in.Change
	target, err := p.resolveTarget(c.Target)
	if err != nil {
		return c, err
	}
	instance, ok := p.m.Targets[target].Instances[c.Name]
	if !ok {
		return c, invalid("target %s declares no instance named %q", target, c.Name)
	}
	if err = p.hostSets(target); err != nil {
		return c, err
	}
	c.Target, c.Kind, c.Dir = target, p.m.Targets[target].Agent, instance.Dir
	return c, nil
}

// resolveTarget accepts a target name or a kind used by exactly one target.
func (p *planner) resolveTarget(ref string) (string, error) {
	if _, ok := p.m.Targets[ref]; ok {
		return ref, nil
	}
	targets := p.targetsOf(ref)
	switch len(targets) {
	case 1:
		return targets[0], nil
	case 0:
		return "", invalid("no target or agent kind named %q", ref)
	}
	return "", invalid("agent %s is used by targets %s; name the target instead", ref, strings.Join(targets, ", "))
}

// hostSets refuses targets whose instance set a schema-1 host table
// replaces: an all-hosts edit would not reach those hosts.
func (p *planner) hostSets(target string) error {
	for _, host := range slices.Sorted(maps.Keys(p.m.Hosts)) {
		if p.m.Hosts[host].Targets[target].Instances != nil {
			return unsupported("[hosts.%s.targets.%s.instances] replaces this target's instances on %s; agent edits change all hosts only, so edit kitbag.toml by hand until per-host instances arrive with manifest schema 2", host, target, host)
		}
	}
	return nil
}

// dir normalizes a directory to the manifest's ~/ form below HOME.
func (p *planner) dir(value string) (string, error) {
	path := value
	switch {
	case value == "~":
		return "", invalid("the home directory itself cannot be an agent directory")
	case strings.HasPrefix(value, "~/"):
		path = filepath.Join(p.home, filepath.FromSlash(value[2:]))
	case !filepath.IsAbs(value):
		return "", invalid("directory %q must start with ~/ or be an absolute path below the home directory", value)
	}
	rel, err := filepath.Rel(p.home, filepath.Clean(path))
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", invalid("directory %s is outside the home directory %s; a manifest can declare only directories below HOME", value, p.home)
	}
	dir := "~/" + filepath.ToSlash(rel)
	if err = manifest.CheckHomePath(dir); err != nil {
		return "", invalid("directory %s: %v", value, err)
	}
	return dir, nil
}

// unique refuses a directory another instance of the same kind declares,
// compared case-insensitively like the manifest's identity rule.
func (p *planner) unique(kind, dir, target, name string) error {
	if err := p.hostSets(target); err != nil {
		return err
	}
	for _, t := range slices.Sorted(maps.Keys(p.m.Targets)) {
		if p.m.Targets[t].Agent != kind {
			continue
		}
		for _, n := range slices.Sorted(maps.Keys(p.m.Targets[t].Instances)) {
			if (t != target || n != name) && strings.EqualFold(p.m.Targets[t].Instances[n].Dir, dir) {
				return invalid("%s is already declared as %s/%s", dir, t, n)
			}
		}
	}
	return nil
}

func (p *planner) settings(target string, value *bool, created bool) error {
	if value != nil && (created || p.m.Targets[target].Settings == "") {
		return invalid("target %s has no settings fragment to merge; --settings applies only to targets that declare settings", target)
	}
	return nil
}

// observe warns about a directory that sync cannot use yet. It refuses a
// non-directory, which no agent can use.
func (p *planner) observe(dir string) error {
	path := filepath.Join(p.home, filepath.FromSlash(dir[2:]))
	info, err := p.in.Host.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		p.warn("%s does not exist; sync warns and skips it until the agent creates it", dir)
	case err != nil:
		p.warn("%s cannot be inspected: %v", dir, err)
	case info.Mode()&fs.ModeSymlink != 0:
		p.warn("%s is a symbolic link; check the result with kitbag plan", dir)
	case !info.IsDir():
		return invalid("%s exists and is not a directory", dir)
	}
	return nil
}

func on(value *bool) bool { return value == nil || *value }
