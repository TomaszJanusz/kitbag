// Package embedded implements basic Git operations in Go, without a git executable.
package embedded

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded/sshauth"
)

// go-git has a process-wide transport registry. Install its Go file transport
// once before any embedded network operation; its default shells out to git.
var installFileTransport sync.Once //nolint:gochecknoglobals // go-git's global registry requires one synchronized installation.

// Options configure the backend without reading the process environment.
type Options struct {
	Env platform.Env
	// SSH supplies the filesystem and optional prompt/keyring services.
	SSH sshauth.Options
	// Identity is the fallback when Git configuration has no complete identity.
	Identity vcs.Identity
	// NoSystemConfig isolates tests from /etc/gitconfig and /etc/ssh/ssh_config.
	NoSystemConfig bool
}

// Backend implements vcs.Backend. Construct it with New.
type Backend struct{ opts Options }

var _ vcs.Backend = (*Backend)(nil)

// New constructs a backend; it never probes for or starts system Git.
func New(opts Options) *Backend {
	installFileTransport.Do(func() {
		client.InstallProtocol("file", newLocalClient())
		// Like the file transport, SSH must never consult go-git's process HOME.
		// Freeze this default once, before any embedded operation starts.
		gitssh.DefaultSSHConfig = nil
		client.InstallProtocol("ssh", sshTransport{Transport: gitssh.NewClient(nil)})
	})
	return &Backend{opts: opts}
}

// Kind returns vcs.Embedded.
func (*Backend) Kind() vcs.Kind { return vcs.Embedded }

// Version returns the pinned go-git version.
func (*Backend) Version() string { return "go-git v5.19.3" }

func checkDir(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return fmt.Errorf("directory %q: %w", dir, vcs.ErrInvalidArgument)
	}
	return nil
}

func checkName(value string) error {
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsRune(value, 0) {
		return fmt.Errorf("name %q: %w", value, vcs.ErrInvalidArgument)
	}
	return nil
}

func branchName(name string) (plumbing.ReferenceName, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	ref := plumbing.NewBranchReferenceName(name)
	if err := ref.Validate(); err != nil {
		return "", fmt.Errorf("branch %q: %w: %w", name, vcs.ErrInvalidArgument, err)
	}
	return ref, nil
}

func open(ctx context.Context, dir string) (*git.Repository, error) {
	if err := checkDir(ctx, dir); err != nil {
		return nil, err
	}
	return git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: false})
}

// Head reads the current branch and commit, including an unborn branch.
func (*Backend) Head(ctx context.Context, dir string) (vcs.Head, error) {
	r, err := open(ctx, dir)
	if err != nil {
		return vcs.Head{}, err
	}
	return head(r)
}

func head(r *git.Repository) (vcs.Head, error) {
	raw, err := r.Reference(plumbing.HEAD, false)
	if err != nil {
		return vcs.Head{}, err
	}
	h := vcs.Head{}
	if raw.Type() == plumbing.SymbolicReference {
		h.Branch = raw.Target().Short()
	}
	ref, err := r.Head()
	if err == nil {
		h.Commit = ref.Hash().String()
	} else if h.Branch == "" || !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return h, err
	}
	return h, nil
}

// RemoteURL returns a configured remote URL verbatim.
func (*Backend) RemoteURL(ctx context.Context, dir, remote string) (string, error) {
	if err := checkName(remote); err != nil {
		return "", err
	}
	r, err := open(ctx, dir)
	if err != nil {
		return "", err
	}
	cfg, err := r.Config()
	if err != nil {
		return "", err
	}
	rc := cfg.Remotes[remote]
	if rc == nil || len(rc.URLs) == 0 {
		return "", fmt.Errorf("remote %s: %w", remote, vcs.ErrNoRemote)
	}
	return rc.URLs[0], nil
}
