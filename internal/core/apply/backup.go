package apply

import (
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func backupAvailable(fsys platform.FS, path string) error {
	entries, err := fsys.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	prefix := strings.ToLower(filepath.Base(path) + ".before-kitbag.")
	legacy := filepath.Base(path) + ".kitbag-backup"
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), prefix) || strings.EqualFold(entry.Name(), legacy) {
			return errChanged()
		}
	}
	return nil
}
