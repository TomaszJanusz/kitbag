package initialize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// Service holds first-run dependencies. Config must describe the requested
// clone, and Adapters supplies the same adapter factory as everyday planning.
type Service struct {
	Host              platform.Host
	Backend           vcs.Backend
	Adapters          workflow.AdapterFactory
	Filename, Version string
	Config            config.Config
}

// ConfigPath resolves the flag, environment, then default configuration path.
// Explicit relative paths must be made absolute by the command layer.
func ConfigPath(host platform.Env, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if value, ok := host.LookupEnv("KITBAG_CONFIG"); ok && value != "" {
		return value, nil
	}
	dirs, err := host.Dirs()
	return filepath.Join(dirs.Config, "config.toml"), err
}

// Exists recognizes every existing entry, including invalid files and dangling
// symlinks. Init never parses or replaces an existing configuration.
func Exists(host platform.FS, path string) (bool, error) {
	_, err := host.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Clone validates intent before making a clone. An interrupted setup can reuse
// only a real directory with the exact requested origin and branch.
func (s Service) Clone(ctx context.Context) (string, bool, error) {
	if _, err := config.Encode(s.Config); err != nil {
		return "", false, err
	}
	repo, err := s.Config.WorkingCopy(s.Host)
	if err != nil {
		return "", false, err
	}
	info, err := s.Host.Lstat(repo)
	if errors.Is(err, fs.ErrNotExist) {
		err = s.Backend.Clone(ctx, s.Config.Repo.URL, repo, vcs.CloneOptions{Branch: s.Config.Repo.Branch})
		return repo, false, err
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("working copy is not a real directory: %w", fs.ErrExist)
	}
	return repo, true, s.checkClone(ctx, repo)
}

func (s Service) checkClone(ctx context.Context, repo string) error {
	origin, err := s.Backend.RemoteURL(ctx, repo, vcs.DefaultRemote)
	if err != nil {
		return err
	}
	if origin != s.Config.Repo.URL {
		return fmt.Errorf("working copy %s has origin %q, not %q: %w", repo, origin, s.Config.Repo.URL, fs.ErrExist)
	}
	head, err := s.Backend.Head(ctx, repo)
	if err != nil {
		return err
	}
	if s.Config.Repo.Branch != "" && head.Branch != s.Config.Repo.Branch {
		return fmt.Errorf("working copy %s is on branch %q, not %q: %w", repo, head.Branch, s.Config.Repo.Branch, fs.ErrExist)
	}
	return nil
}

// NeedsIdentity asks only when the effective Git configuration has no identity.
// Clone and sync do not create commits; a headless consumer may omit identity.
func (s Service) NeedsIdentity(ctx context.Context, repo string) (bool, error) {
	inspector, ok := s.Backend.(vcs.CommitInspector)
	if !ok {
		return true, nil
	}
	cfg, err := inspector.InspectCommit(ctx, repo)
	return !cfg.Identity, err
}

// Inspect uses the ordinary dry plan pipeline. It neither applies targets nor
// edits Git includes; planner reads of managed destinations are distinct from
// directory-only agent detection.
func (s Service) Inspect(repo string) (workflow.Inspection, error) {
	store, err := state.New(s.Host, s.Host, state.Options{})
	if err != nil {
		return workflow.Inspection{}, err
	}
	service := workflow.Service{Host: s.Host, Backend: s.Backend, Store: store, Config: s.Config, Repository: repo, Version: s.Version, Adapters: s.Adapters}
	return service.Inspect()
}

// Save installs configuration after the UI has displayed the plan and obtained
// confirmation. Existing files always win, including a competing init's file.
func (s Service) Save() error { return config.Create(s.Host, s.Filename, s.Config) }
