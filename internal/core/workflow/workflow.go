// Package workflow coordinates everyday commands through injected host and Git contracts.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// AdapterFactory constructs only the adapters required by the resolved manifest.
// Warning messages may be appended while the planner calls those adapters.
type AdapterFactory func(*manifest.Manifest, *[]string) ([]target.Adapter, error)

// Service supplies command dependencies. Now is required for Sync only.
type Service struct {
	Host       platform.Host
	Backend    vcs.Backend
	Store      *state.Store
	Adapters   AdapterFactory
	Repository string
	Version    string
	HostFlag   string
	ModeFlag   string
	Now        func() time.Time
	Config     config.Config
}

// SyncResult retains only work performed in this single pass.
type SyncResult struct {
	Apply    apply.Result
	Includes vcs.IncludeResult
	Warnings []string
	Pull     vcs.PullOutcome
}

// Code returns the report's stable exit classification.
func (r SyncResult) Code() apply.Code {
	code := r.Apply.Code()
	if len(r.Warnings) > 0 || len(r.Includes.Warnings) > 0 {
		code = max(code, apply.Warnings)
	}
	if r.Pull == vcs.PullDiverged {
		code = max(code, apply.Conflict)
	}
	return code
}

// Sync serializes pull → new manifest → version check → one plan → one apply.
// Holding the same lock for all steps also serializes timer and manual runs.
func (s Service) Sync(ctx context.Context, noPull, force bool) (SyncResult, error) {
	var result SyncResult
	if s.Now == nil || s.Now().IsZero() {
		return result, fmt.Errorf("sync clock: %w", invalidInput())
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		if _, _, err := session.LoadSync(); err != nil {
			return err
		}
		runErr := s.syncLocked(ctx, session, noPull, force, &result)
		code := max(result.Code(), apply.ErrorCode(runErr))
		record := state.SyncRecord{SchemaVersion: 1, At: s.Now(), Repository: s.Repository, Result: int(code), Changed: len(result.Apply.Changed) + len(result.Includes.Added)}
		return errors.Join(runErr, session.SaveSync(record))
	})
	return result, err
}

func (s Service) syncLocked(ctx context.Context, session *state.Session, noPull, force bool, result *SyncResult) error {
	if err := s.ensureWorkingCopy(ctx, noPull); err != nil {
		return err
	}
	if err := s.checkBranch(ctx); err != nil {
		return err
	}
	if !noPull {
		pulled, err := vcs.Pull(ctx, s.Backend, s.Repository)
		result.Pull = pulled.Outcome
		if err != nil {
			return err
		}
		if pulled.Outcome == vcs.PullDiverged {
			return nil
		}
	}
	m, mode, err := s.intent()
	if err != nil {
		return err
	}
	if err = apply.Recover(s.Host, session); err != nil {
		return err
	}
	p, err := s.build(session, m, mode, force, &result.Warnings)
	if err != nil {
		return err
	}
	result.Apply, err = apply.Execute(s.Host, session, p, apply.Options{Mode: mode, Force: force, Now: s.Now})
	if err != nil {
		return err
	}
	result.Includes, err = s.includes(ctx, m)
	return err
}

func (s Service) includes(ctx context.Context, m *manifest.Manifest) (vcs.IncludeResult, error) {
	if m.Git == nil {
		return vcs.IncludeResult{}, nil
	}
	cfg, ok := s.Backend.(vcs.GlobalConfig)
	if !ok {
		return vcs.IncludeResult{Warnings: []string{"selected Git backend cannot manage global includes"}}, nil
	}
	home, err := s.Host.HomeDir()
	if err != nil {
		return vcs.IncludeResult{}, err
	}
	paths := make([]string, 0, len(m.Git.Include))
	for _, include := range m.Git.Include {
		paths = append(paths, filepath.Join(s.Repository, filepath.FromSlash(include)))
	}
	return vcs.EnsureIncludes(ctx, cfg, s.Host, home, paths)
}

func (s Service) intent() (*manifest.Manifest, string, error) {
	m, err := manifest.Load(s.Host, filepath.Join(s.Repository, "kitbag.toml"))
	if err != nil {
		return nil, "", err
	}
	if err = m.CheckVersion(s.Version); err != nil {
		return nil, "", err
	}
	host, _, err := config.Resolve(s.Host, config.Values{"host": s.Config.Host}, "host", s.HostFlag)
	if err != nil {
		return nil, "", err
	}
	m, err = m.ResolveHost(hostEnvironment{s.Host, host}, host)
	if err != nil {
		return nil, "", err
	}
	mode, _, err := config.Resolve(s.Host, config.Values{"sync.mode": s.Config.Sync.Mode}, "sync.mode", s.ModeFlag)
	return m, mode, err
}

func (s Service) build(session *state.Session, m *manifest.Manifest, mode string, force bool, warnings *[]string) (plan.Plan, error) {
	db, err := session.Load()
	if err != nil {
		return plan.Plan{}, err
	}
	req, err := s.request(m, mode, warnings)
	if err != nil {
		return plan.Plan{}, err
	}
	req.State, req.Force = db, force
	return plan.Build(s.Host, req)
}

// request assembles planner input without state: adapters for the resolved
// manifest and the explicitly configured legacy roots expanded through HOME.
func (s Service) request(m *manifest.Manifest, mode string, warnings *[]string) (plan.Request, error) {
	adapters, err := s.Adapters(m, warnings)
	if err != nil {
		return plan.Request{}, err
	}
	home, err := s.Host.HomeDir()
	if err != nil {
		return plan.Request{}, err
	}
	roots := make([]string, 0, len(s.Config.Migration.LegacyRoots))
	for _, root := range s.Config.Migration.LegacyRoots {
		roots = append(roots, config.Expand(home, root))
	}
	return plan.Request{Repository: s.Repository, Manifest: m, Mode: mode, State: state.Database{SchemaVersion: state.CurrentVersion}, Adapters: adapters, LegacyRoots: roots}, nil
}

type hostEnvironment struct {
	platform.Env
	flag string
}

// LookupEnv gives the explicit host flag priority over the environment.
func (e hostEnvironment) LookupEnv(key string) (string, bool) {
	if key == "KITBAG_HOST" && e.flag != "" {
		return e.flag, true
	}
	return e.Env.LookupEnv(key)
}

func (s Service) checkBranch(ctx context.Context) error {
	if s.Config.Repo.Branch == "" {
		return nil
	}
	status, err := s.Backend.Status(ctx, s.Repository)
	if err != nil {
		return err
	}
	if status.Branch != s.Config.Repo.Branch {
		return fmt.Errorf("working copy branch %q differs from repo.branch %q; switch it explicitly: %w", status.Branch, s.Config.Repo.Branch, invalidInput())
	}
	return nil
}

// A first sync owns clone creation; --no-pull requires an existing worktree.
func (s Service) ensureWorkingCopy(ctx context.Context, noPull bool) error {
	_, err := s.Host.Lstat(s.Repository)
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if noPull {
		return fmt.Errorf("working copy missing with --no-pull: %w", err)
	}
	return s.Backend.Clone(ctx, s.Config.Repo.URL, s.Repository, vcs.CloneOptions{Branch: s.Config.Repo.Branch})
}
