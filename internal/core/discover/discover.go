// Package discover resolves agent instance candidates: each native adapter's
// default configuration directory under the caller's relocation variables. A
// candidate is observed with one Lstat of the directory itself; nothing is
// opened or followed; HOME names are listed only by an explicit List probe (ADR 12, section 1). The adapters' resolvers
// arrive as an injected Table, so core imports no adapter.
package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// ErrUnsupported reports an agent kind without a resolver in the table.
var ErrUnsupported = errors.New("unsupported agent")

// Source says where a candidate's directory comes from.
type Source string

// Candidate sources. Declared and probed candidates arrive with the commands
// that list them.
const (
	// Default is the adapter's documented directory below HOME.
	Default Source = "default"
	// Environment is a relocation variable set by the caller.
	Environment Source = "environment"
	// Declared is an explicit manifest instance.
	Declared Source = "declared"
	// Probe is an opt-in HOME name match.
	Probe Source = "probe"
)

// Presence is what a single Lstat found at a candidate's directory.
type Presence string

// Presence values; only Present is a usable directory.
const (
	Present      Presence = "present"
	Missing      Presence = "missing"
	NotDirectory Presence = "not_directory"
	// Symlink is a final symbolic link, reported and never followed.
	Symlink Presence = "symlink"
	// Unreadable means Lstat failed for another reason; see Candidate.Err.
	Unreadable Presence = "unreadable"
	// OutsideHome is a directory that cannot be expressed in a manifest.
	OutsideHome Presence = "outside_home"
)

// Host is the part of the platform discovery needs: the caller's environment
// and Lstat. It deliberately has no way to read or list files.
type Host interface {
	platform.Env
	Lstat(path string) (fs.FileInfo, error)
}

// Resolver is one agent kind's default-directory rule.
type Resolver struct {
	// Home resolves the default directory. Nil means the kind has no default
	// and is used only through listed instances.
	Home func(platform.Env) (string, error)
	// Agent is the manifest agent kind.
	Agent string
	// ProbePrefix is a documented direct HOME directory name. Empty disables probing.
	ProbePrefix string
	// Variable names the relocation variable, if any; it only labels the
	// source, as Home applies it.
	Variable string
	// DefaultDir is the documented default as a manifest home path
	// ("~/.claude"), independent of relocation variables; empty without a
	// default. Agent edits declare it when a target's implicit default
	// would otherwise stop being managed.
	DefaultDir string
}

// Table holds the resolvers in presentation order.
type Table []Resolver

// Only returns the resolvers of the given kinds, keeping the table's order.
func (t Table) Only(agents ...string) Table {
	return slices.DeleteFunc(slices.Clone(t), func(r Resolver) bool { return !slices.Contains(agents, r.Agent) })
}

// Home returns agent's default directory, or "" when the kind has none. An
// explicit instance dir always wins; callers use Home only for targets that
// list no instances.
func (t Table) Home(env platform.Env, agent string) (string, error) {
	for _, r := range t {
		if r.Agent != agent {
			continue
		}
		if r.Home == nil {
			return "", nil
		}
		return r.Home(env)
	}
	return "", fmt.Errorf("%w %s", ErrUnsupported, agent)
}

// Candidate is a resolved default directory and its presence.
type Candidate struct {
	// Err is the Lstat error when Presence is Unreadable.
	Err              error
	Agent, Directory string
	Source           Source
	// Variable names the relocation variable when Source is Environment.
	Variable string
	Presence Presence
}

// Defaults resolves and observes every default directory in table order.
// A resolution error, such as a relative relocation variable, stops it.
func (t Table) Defaults(host Host) ([]Candidate, error) {
	result := []Candidate{}
	for _, r := range t {
		if r.Home == nil {
			continue
		}
		dir, err := r.Home(host)
		if err != nil {
			return nil, err
		}
		candidate := Candidate{Agent: r.Agent, Directory: dir, Source: Default}
		if relocated(host, r.Variable) {
			candidate.Source, candidate.Variable = Environment, r.Variable
		}
		candidate.Presence, candidate.Err = observe(host, dir)
		result = append(result, candidate)
	}
	return result, nil
}

// relocated reports whether variable holds an absolute path. Resolvers reject
// or ignore (XDG) any other non-empty value, so it then selected the default.
func relocated(env platform.Env, variable string) bool {
	if variable == "" {
		return false
	}
	value, _ := env.LookupEnv(variable)
	return filepath.IsAbs(value)
}

func observe(host Host, dir string) (Presence, error) {
	info, err := host.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Missing, nil
	case err != nil:
		return Unreadable, err
	case info.Mode()&fs.ModeSymlink != 0:
		return Symlink, nil
	case info.IsDir():
		return Present, nil
	default:
		return NotDirectory, nil
	}
}
