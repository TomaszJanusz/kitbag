package workflow

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
	"github.com/TomaszJanusz/kitbag/internal/core/undo"
	"github.com/TomaszJanusz/kitbag/internal/target"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// AgentEffect is a read-only filesystem consequence of the proposed manifest.
// Kind uses the plan operation vocabulary, plus unchanged for in-sync paths.
type AgentEffect struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Backup   string `json:"backup,omitempty"`
	Warning  string `json:"warning,omitempty"`
	Existing bool   `json:"existing"`
}

func (s Service) agentEffects(result *AgentResult) error {
	home, err := s.Host.HomeDir()
	if err != nil {
		return err
	}
	result.Directory = config.Expand(home, result.Plan.Change.Dir)
	// Limited callers can preview manifest edits without filesystem planning.
	// Nil effects explicitly report that those details are unavailable.
	if s.Adapters == nil || s.Store == nil {
		return nil
	}
	m, mode, err := s.agentEffectIntent(result)
	if err != nil {
		return err
	}
	req, err := s.request(m, mode, &result.Plan.Warnings)
	if err != nil {
		return err
	}
	after, err := plan.Declarations(s.Host, req)
	if err != nil {
		return err
	}
	only := droppedPaths(config.Expand(home, result.Plan.Dropped), after)
	return s.Store.WithLock(func(session *state.Session) error {
		p, buildErr := s.build(session, m, mode, false, &result.Plan.Warnings)
		if buildErr != nil {
			return buildErr
		}
		if result.Plan.Dropped != "" {
			p, buildErr = s.agentReleaseEffects(session, p, only)
			if buildErr != nil {
				return buildErr
			}
		}
		result.Effects, buildErr = s.effects(p)
		return buildErr
	})
}

// droppedPaths is shared by previews and apply: a retained declaration must
// never be released, even when its path lies inside a dropped instance root.
func droppedPaths(root string, after []target.Target) func(string) bool {
	kept := map[string]bool{}
	for _, d := range after {
		kept[d.Path] = true
	}
	return func(path string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && filepath.IsLocal(rel) && !kept[path]
	}
}

func (s Service) agentReleaseEffects(session *state.Session, p plan.Plan, only func(string) bool) (plan.Plan, error) {
	before, err := s.declarations()
	if err != nil {
		return p, err
	}
	roots, err := s.legacyRoots()
	if err != nil {
		return p, err
	}
	outcome, err := undo.Unmanage(s.Host, session, before, undo.Options{DryRun: true, ReleaseChanged: true, Only: only, LegacyRoots: roots})
	if err != nil {
		return p, err
	}
	operations := make([]plan.Operation, 0, len(p.Operations)+len(outcome.Plan.Operations))
	for _, op := range p.Operations {
		if !only(op.Path) {
			operations = append(operations, op)
		}
	}
	operations = append(operations, outcome.Plan.Operations...)
	p.Operations = operations
	return p, nil
}

func (s Service) effects(p plan.Plan) ([]AgentEffect, error) {
	effects := make([]AgentEffect, 0, len(p.Operations)+len(p.Observations))
	seen := map[string]bool{}
	for _, op := range p.Operations {
		_, err := s.Host.Lstat(op.Path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		effects = append(effects, AgentEffect{Path: op.Path, Kind: string(op.Kind), Backup: op.BackupPath, Warning: op.Warning, Existing: err == nil})
		seen[op.Path] = true
	}
	for _, observation := range p.Observations {
		if !seen[observation.Path] && observation.Classification == plan.InSync {
			effects = append(effects, AgentEffect{Path: observation.Path, Kind: "unchanged", Existing: true})
		}
	}
	return effects, nil
}

func (s Service) agentEffectIntent(result *AgentResult) (*manifest.Manifest, string, error) {
	m, err := manifest.Parse(result.Manifest, result.Plan.After)
	if err != nil {
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
