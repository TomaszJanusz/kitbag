// Package agents plans changes to the agent instances declared in
// kitbag.toml: it validates a requested add, edit or remove against the
// manifest, the adapters' discovery table and the host, and renders the
// position-preserving edit, its diff and a generated Conventional Commit
// message. It writes nothing; workflow publishes a plan (ADR 12, v1 slice).
package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Op is the kind of change.
type Op string

// Operations.
const (
	Add    Op = "add"
	Edit   Op = "edit"
	Remove Op = "remove"
)

// Change is one requested instance change. Add needs Kind, Name and Dir;
// edit and remove name the instance by Target (a target name, or an agent kind
// used by exactly one target) and Name. Settings nil keeps the default (add)
// or the current value (edit). Dir accepts ~/..., ~ or an absolute path below
// HOME and is normalized to the ~/ form.
type Change struct {
	Settings *bool  `json:"settings,omitempty"`
	Op       Op     `json:"op"`
	Kind     string `json:"kind,omitempty"`
	Target   string `json:"target,omitempty"`
	Name     string `json:"name"`
	Dir      string `json:"dir,omitempty"`
}

// Host is what planning reads from the machine: HOME, relocation variables
// and one Lstat of the instance directory, never its contents.
type Host interface {
	HomeDir() (string, error)
	LookupEnv(key string) (string, bool)
	Lstat(path string) (fs.FileInfo, error)
}

// Input is everything Prepare needs.
type Input struct {
	Host   Host
	Change Change
	// Filename labels manifest diagnostics.
	Filename string
	// HostName is recorded in the Kitbag-Host commit trailer.
	HostName string
	Manifest []byte
	// Kinds lists the native agent kinds; generic is refused (it needs roots).
	Kinds discover.Table
}

// Plan is a validated change that has not been written.
type Plan struct {
	// Change is normalized: Kind and Target resolved, Dir in ~/ form.
	Change Change `json:"change"`
	// Revision identifies the manifest bytes the plan was made from.
	Revision string `json:"revision"`
	// Title is the generated commit title; Message adds the body and trailers.
	Title   string `json:"title"`
	Message string `json:"message"`
	// Dropped is the ~/ directory the change stops declaring (remove, or an
	// edit that moves the instance); its KitBag outputs are released.
	Dropped string `json:"dropped_dir,omitempty"`
	// Diff is a unified diff of kitbag.toml, one line per entry.
	Diff     []string `json:"diff"`
	Warnings []string `json:"warnings"`
	// Edits are the structural edits, After the edited manifest.
	Edits []manifestedit.Edit `json:"-"`
	After []byte              `json:"-"`
}

// Errors returned (wrapped) by Prepare. Both also match fs.ErrInvalid, so the
// CLI reports them as usage errors (exit 2).
var (
	// ErrInvalid is a change the manifest or this machine rules out.
	ErrInvalid = errors.New("invalid agent change")
	// ErrUnsupported is a change outside the v1 slice, such as a host scope.
	ErrUnsupported = errors.New("not supported by agent edits yet")
)

// usageError is a validation message classified as kind and fs.ErrInvalid.
type usageError struct {
	kind, cause error
	message     string
}

func (e *usageError) Error() string { return e.message }
func (e *usageError) Unwrap() []error {
	if e.cause != nil {
		return []error{e.kind, fs.ErrInvalid, e.cause}
	}
	return []error{e.kind, fs.ErrInvalid}
}

func invalid(format string, args ...any) error {
	return &usageError{kind: ErrInvalid, message: fmt.Sprintf(format, args...)}
}

func unsupported(format string, args ...any) error {
	return &usageError{kind: ErrUnsupported, message: fmt.Sprintf(format, args...)}
}

// Revision returns the identifier of manifest bytes used for compare-and-swap.
func Revision(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Prepare validates in.Change and returns the edit, diff and commit message.
func Prepare(in Input) (Plan, error) {
	m, err := manifest.Parse(in.Filename, in.Manifest)
	if err != nil {
		return Plan{}, err
	}
	home, err := in.Host.HomeDir()
	if err != nil {
		return Plan{}, err
	}
	p := &planner{in: in, m: m, home: home, plan: Plan{Revision: Revision(in.Manifest), Warnings: []string{}}}
	if err = p.prepare(); err != nil {
		return Plan{}, err
	}
	after, err := manifestedit.Apply(in.Filename, in.Manifest, p.plan.Edits...)
	if err != nil {
		if errors.Is(err, manifestedit.ErrLayout) {
			err = &usageError{kind: ErrUnsupported, message: err.Error(), cause: err}
		}
		return Plan{}, err
	}
	p.plan.After = after
	p.plan.Diff = Diff(in.Manifest, after)
	p.plan.Title, p.plan.Message = message(p.plan.Change, p.plan.Edits, in.HostName)
	return p.plan, nil
}

type planner struct {
	in   Input
	m    *manifest.Manifest
	home string
	plan Plan
}

func (p *planner) prepare() error {
	switch p.in.Change.Op {
	case Add:
		return p.add()
	case Edit:
		return p.edit()
	case Remove:
		return p.remove()
	default:
		return invalid("unknown operation %q (want add, edit or remove)", p.in.Change.Op)
	}
}

func (p *planner) warn(format string, args ...any) {
	p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf(format, args...))
}
