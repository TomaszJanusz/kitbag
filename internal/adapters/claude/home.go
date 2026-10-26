package claude

import "github.com/TomaszJanusz/kitbag/internal/platform"

// Agent is the manifest agent kind this adapter serves.
const Agent = "claude"

// Default location and its relocation variable; HomeVariable relocates
// the whole default configuration directory.
const (
	HomeVariable = "CLAUDE_CONFIG_DIR"
	DefaultHome  = ".claude"
)

// Home resolves the default Claude configuration directory: CLAUDE_CONFIG_DIR
// when set (it must be absolute), otherwise ~/.claude. Listed instances never
// use this rule.
func Home(env platform.Env) (string, error) {
	return env.AgentHome(HomeVariable, DefaultHome)
}
