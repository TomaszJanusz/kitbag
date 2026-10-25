package workflow

import (
	"fmt"

	"github.com/TomaszJanusz/kitbag/internal/core/adopt"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
)

// legacyRootsHint explains the open migration question instead of guessing paths.
const legacyRootsHint = `adopt needs the old agent-config clone and shared skill roots: add them to config.toml, for example
[migration]
legacy_roots = ["~/src/agent-config", "~/.agents/skills"]
(review the actual paths first; see docs/adopt.md)`

// AdoptResult is an adoption report plus nonblocking adapter warnings.
type AdoptResult struct {
	Warnings []string
	Report   adopt.Report
}

// Adopt records ownership of destinations that already hold the desired output
// (zero file changes) without fetching. MoveRealDirs moves real skill
// directories aside once. A dry run reads only.
func (s Service) Adopt(moveRealDirs, dryRun bool) (AdoptResult, error) {
	result := AdoptResult{Warnings: []string{}}
	if err := s.compatible(); err != nil {
		return result, err
	}
	if len(s.Config.Migration.LegacyRoots) == 0 {
		return result, fmt.Errorf("%s: %w", legacyRootsHint, invalidInput())
	}
	if s.Now == nil || s.Now().IsZero() {
		return result, fmt.Errorf("adopt clock: %w", invalidInput())
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		m, mode, err := s.intent()
		if err != nil {
			return err
		}
		req, err := s.request(m, mode, &result.Warnings)
		if err != nil {
			return err
		}
		home, err := s.Host.HomeDir()
		if err != nil {
			return err
		}
		result.Report, err = adopt.Run(s.Host, session, req, adopt.Options{Now: s.Now, Home: home, Mode: mode, MoveRealDirs: moveRealDirs, DryRun: dryRun})
		return err
	})
	return result, err
}

// LifecycleResult is an unmanage or revert outcome plus adapter warnings.
type LifecycleResult struct {
	Warnings []string
	Outcome  undo.Outcome
}

// Unmanage removes KitBag's own outputs and restores displaced originals. It
// reads the manifest only to find settings mergers; it never fetches.
func (s Service) Unmanage(dryRun bool) (LifecycleResult, error) {
	result := LifecycleResult{Warnings: []string{}}
	if err := s.compatible(); err != nil {
		return result, err
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		m, mode, err := s.intent()
		if err != nil {
			return err
		}
		req, err := s.request(m, mode, &result.Warnings)
		if err != nil {
			return err
		}
		declarations, err := plan.Declarations(s.Host, req)
		if err != nil {
			return err
		}
		result.Outcome, err = undo.Unmanage(s.Host, session, declarations, undo.Options{Now: s.Now, DryRun: dryRun})
		return err
	})
	return result, err
}

// Revert restores the destinations changed by the latest to journaled plans.
// It needs neither the manifest nor the working copy.
func (s Service) Revert(to int, force, dryRun bool) (LifecycleResult, error) {
	result := LifecycleResult{Warnings: []string{}}
	if err := s.compatible(); err != nil {
		return result, err
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		var err error
		result.Outcome, err = undo.Revert(s.Host, session, to, undo.Options{Now: s.Now, Force: force, DryRun: dryRun})
		return err
	})
	return result, err
}

// History lists journaled plans, newest first, without changing anything.
func (s Service) History() ([]undo.Applied, error) {
	var history []undo.Applied
	if err := s.compatible(); err != nil {
		return nil, err
	}
	err := s.Store.WithLock(func(session *state.Session) error {
		var err error
		history, err = undo.List(session)
		return err
	})
	return history, err
}
