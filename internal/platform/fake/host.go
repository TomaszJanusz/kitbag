package fake

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"sync"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
	"github.com/TomaszJanusz/kitbag/internal/platform/internal/xdg"
)

// defaultHostname is reported unless [WithHostname] says otherwise.
const defaultHostname = "fake-host"

var errMountNotEmpty = errors.New("mount point is not an empty directory")

// Host is an in-memory platform.Host. Create it with [New].
type Host struct {
	listeners  map[string]*localListener
	volumes    map[string]*node
	locks      map[*node]bool
	env        map[string]string
	terminals  map[uintptr]bool
	writeFault *writeFault
	hostname   string
	opened     []string
	devices    int
	mu         sync.Mutex
}

var _ platform.Host = (*Host)(nil)

// Option configures a [Host].
type Option func(*Host)

// WithEnv sets the environment; by default it is empty, so HOME is unset.
func WithEnv(env map[string]string) Option {
	return func(h *Host) {
		h.env = maps.Clone(env)
	}
}

// WithHostname sets the host name reported by Hostname.
func WithHostname(name string) Option {
	return func(h *Host) {
		h.hostname = name
	}
}

// WithTerminal marks file descriptors that IsTerminal reports as terminals.
func WithTerminal(fds ...uintptr) Option {
	return func(h *Host) {
		for _, fd := range fds {
			h.terminals[fd] = true
		}
	}
}

// New returns an empty host whose filesystem holds only the root directory.
func New(opts ...Option) *Host {
	h := &Host{
		listeners: map[string]*localListener{},
		volumes:   map[string]*node{},
		locks:     map[*node]bool{},
		env:       map[string]string{},
		hostname:  defaultHostname,
		terminals: map[uintptr]bool{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// volume returns the root directory of a volume ("" on Unix), creating it.
func (h *Host) volume(name string) *node {
	root, ok := h.volumes[name]
	if !ok {
		root = newDir(rootPerm, 0)
		h.volumes[name] = root
	}
	return root
}

// Mount turns the existing empty directory at path into the root of a new
// simulated filesystem: renames into or out of it fail with
// platform.ErrCrossDevice.
func (h *Host) Mount(path string) error {
	if err := check.Path("mount", path); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.existing("mount", path, true)
	if err != nil {
		return err
	}
	if !n.isDir() || len(n.children) > 0 {
		return check.Invalid("mount", path, errMountNotEmpty)
	}
	h.devices++
	n.dev = h.devices
	return nil
}

// Opened returns the targets passed to Open, in order.
func (h *Host) Opened() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.opened)
}

// LookupEnv returns the value of a configured environment variable.
func (h *Host) LookupEnv(key string) (string, bool) {
	value, ok := h.env[key]
	return value, ok
}

// HomeDir returns HOME, which must be absolute.
func (h *Host) HomeDir() (string, error) {
	return xdg.Home(h.LookupEnv)
}

// Dirs returns kitbag's XDG directories.
func (h *Host) Dirs() (platform.Dirs, error) {
	return xdg.Dirs(h.LookupEnv)
}

// RuntimeDir returns the private local socket directory.
func (h *Host) RuntimeDir() (string, error) {
	return xdg.RuntimeDir(h.LookupEnv)
}

// AgentHome resolves an agent's home directory.
func (h *Host) AgentHome(override, rel string) (string, error) {
	return xdg.AgentHome(h.LookupEnv, override, rel)
}

// Hostname returns the configured host name.
func (h *Host) Hostname() (string, error) {
	return h.hostname, nil
}

// Open records a validated target without opening anything.
func (h *Host) Open(target string) error {
	if err := check.OpenTarget(target); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.opened = append(h.opened, target)
	return nil
}

// IsTerminal reports whether fd was configured with [WithTerminal].
func (h *Host) IsTerminal(fd uintptr) bool {
	return h.terminals[fd]
}

// existing resolves path to an entry that must exist.
func (h *Host) existing(op, path string, follow bool) (*node, error) {
	w, err := h.walk(path, follow)
	if err == nil && w.node == nil {
		err = fs.ErrNotExist
	}
	if err != nil {
		return nil, wrap(op, path, err)
	}
	return w.node, nil
}

// wrap names the operation and path of a failure.
func wrap(op, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}
