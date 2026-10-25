package plan

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// MaxLegacyHops bounds the intermediate links LinkReaches follows.
const MaxLegacyHops = 2

// LinkReaches reports whether a link at path with text link reaches source:
// its lexical target is source, or a chain of at most MaxLegacyHops further
// links leads there, each of them an entry directly inside a declared legacy
// root (its parent directory is the root). agent-config-sync left such chains,
// for example ~/.claude/skills/x -> ~/.agents/skills/x -> <clone>/skills/x.
// Every hop resolves lexically, as IsManagedLink does; only the intermediate
// entries inside the roots are read, and nothing is followed without roots.
func LinkReaches(fsys platform.FS, path, link, source string, roots []string) (bool, error) {
	resolved := resolve(path, link)
	for hop := 0; resolved != source; hop++ {
		if hop == MaxLegacyHops || !slices.Contains(roots, filepath.Dir(resolved)) {
			return false, nil
		}
		info, err := fsys.Lstat(resolved)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, platform.ErrNotDir) || err == nil && info.Mode()&fs.ModeSymlink == 0 {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		next, err := fsys.ReadLink(resolved)
		if err != nil {
			return false, err
		}
		resolved = resolve(resolved, next)
	}
	return true, nil
}

// resolve returns the clean lexical destination of a link at path with text link.
func resolve(path, link string) string {
	if !filepath.IsAbs(link) {
		link = filepath.Join(filepath.Dir(path), link)
	}
	return filepath.Clean(link)
}
