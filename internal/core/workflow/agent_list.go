package workflow

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/config"
	"github.com/TomaszJanusz/kitbag/internal/core/compatibility"
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// Agents lists the agent kinds, the instances the working copy's manifest
// declares and undeclared candidates (agents_schema 1), like kitbag agent
// list. It reads only; probe adds the explicit HOME name probe.
func (s Service) Agents(probe bool) (discover.Snapshot, error) {
	current, _, err := config.Resolve(s.Host, config.Values{"host": s.Config.Host}, "host", s.HostFlag)
	if err != nil {
		return discover.Snapshot{}, err
	}
	var m *manifest.Manifest
	warning := ""
	_, err = s.Host.Lstat(filepath.Join(s.Repository, manifestName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		warning = "No manifest yet; listing detected agent directories only."
	case err != nil:
		return discover.Snapshot{}, err
	default:
		if m, err = compatibility.Load(s.Host, s.Repository, s.Version); err != nil {
			return discover.Snapshot{}, err
		}
	}
	result, err := s.Kinds.List(s.Host, m, current, probe)
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, err
}

// AgentVariables lists the relocation variables of the known agent kinds,
// the only variables a caller may forward for agents.list.
func (s Service) AgentVariables() []string {
	var keys []string
	for _, r := range s.Kinds {
		if r.Variable != "" {
			keys = append(keys, r.Variable)
		}
	}
	return keys
}
