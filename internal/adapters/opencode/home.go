package opencode

import (
	"path/filepath"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// opencodeHome uses the platform's XDG resolution (including its handling of
// relative and empty values), then selects OpenCode beside KitBag's config.
// OPENCODE_CONFIG_DIR is an additional directory, not a replacement home.
func opencodeHome(env platform.Env) (string, error) {
	dirs, err := env.Dirs()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dirs.Config), Agent), nil
}
