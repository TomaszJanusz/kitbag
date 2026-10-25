package gemini

import (
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// geminiHome composes GEMINI_CLI_HOME's documented parent semantics with
// AgentHome's rejection of relative overrides. Listed instances name the
// configuration directory itself and never use this relocation rule.
func geminiHome(env platform.Env) (string, error) {
	if value, _ := env.LookupEnv(HomeVariable); value != "" {
		parent, err := env.AgentHome(HomeVariable, DefaultHome)
		if err != nil {
			return "", err
		}
		return filepath.Join(parent, DefaultHome), nil
	}
	return env.AgentHome("", DefaultHome)
}
