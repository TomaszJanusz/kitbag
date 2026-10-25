package embedded

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func checkInProgress(dir string) error {
	for _, state := range []struct{ path, feature string }{
		{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"MERGE_HEAD", "non-fast-forward merge"},
	} {
		_, err := os.Lstat(filepath.Join(dir, ".git", state.path))
		if err == nil {
			return unsupported(state.feature + " in progress")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
