// Package initialize coordinates first-run setup through injected host and Git
// contracts. Detection observes directories only; it never reads agent files.
package initialize

import (
	"errors"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Agent is an existing real configuration directory, not proof of an installed
// executable. Environment overrides take precedence over documented defaults.
type Agent struct{ Kind, Directory string }

// Detect conservatively checks only supported adapters' known configuration
// directories. It does not enumerate HOME, follow directory symlinks or inspect
// agent settings, credentials, sessions or instance-list files.
func Detect(host platform.Host) ([]Agent, error) {
	result := []Agent{}
	for _, candidate := range []struct{ kind, variable, relative string }{
		{"claude", "CLAUDE_CONFIG_DIR", ".claude"},
		{"codex", "CODEX_HOME", ".codex"},
	} {
		dir, err := host.AgentHome(candidate.variable, candidate.relative)
		if err != nil {
			return nil, err
		}
		info, err := host.Lstat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return nil, err
		case info.IsDir():
			result = append(result, Agent{Kind: candidate.kind, Directory: dir})
		}
	}
	return result, nil
}
