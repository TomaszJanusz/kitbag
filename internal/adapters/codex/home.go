package codex

import "github.com/TomaszJanusz/kitbag/internal/platform"

// Home resolves the default Codex home: CODEX_HOME when set (it must be
// absolute), otherwise ~/.codex. Listed instances never use this rule.
func Home(env platform.Env) (string, error) {
	return env.AgentHome(HomeVariable, DefaultHome)
}
