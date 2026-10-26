// Package initialize coordinates first-run setup through injected host and Git
// contracts. Detection observes directories only; it never reads agent files.
package initialize

import (
	"github.com/TomaszJanusz/kitbag/internal/core/discover"
	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Agent is an existing real configuration directory, not proof of an installed
// executable. Environment overrides take precedence over documented defaults.
type Agent struct{ Kind, Directory string }

// Detect conservatively checks only the default directories of the kinds init
// can scaffold, resolved by homes. It does not enumerate HOME, follow directory
// symlinks or inspect agent settings, credentials, sessions or instance-list
// files.
func Detect(host platform.Host, homes discover.Table) ([]Agent, error) {
	candidates, err := homes.Only(scaffoldAgents()...).Defaults(host)
	if err != nil {
		return nil, err
	}
	result := []Agent{}
	for _, candidate := range candidates {
		switch candidate.Presence {
		case discover.Unreadable:
			return nil, candidate.Err
		case discover.Present:
			result = append(result, Agent{Kind: candidate.Agent, Directory: candidate.Directory})
		case discover.Missing, discover.NotDirectory, discover.Symlink, discover.OutsideHome:
			// OutsideHome is a listing-only classification; Defaults never returns it.
		}
	}
	return result, nil
}

// scaffoldAgents lists the kinds a new manifest can target, in display order.
func scaffoldAgents() []string { return []string{"claude", "codex"} }
