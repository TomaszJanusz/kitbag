package plan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// HashBytes is the state-compatible, domain-separated digest of file bytes.
func HashBytes(data []byte) string {
	return digest(append([]byte("kitbag-file-v1\x00"), data...))
}

// HashTree hashes regular-file bytes or a directory's sorted, typed child
// digests (including empty directories). Links and special entries are refused;
// permissions and timestamps are intentionally excluded. See docs/plan.md.
func HashTree(fsys platform.FS, path string) (string, error) {
	info, err := fsys.Lstat(path)
	if err != nil {
		return "", err
	}
	switch {
	case info.Mode().IsRegular():
		data, readErr := fsys.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		return HashBytes(data), nil
	case info.IsDir():
		return hashDirectory(fsys, path)
	default:
		return "", fmt.Errorf("unsupported tree entry %s: %w", path, fs.ErrInvalid)
	}
}

type digestEntry struct {
	Name      string `json:"name"`
	Hash      string `json:"hash"`
	Directory bool   `json:"directory"`
}

func hashDirectory(fsys platform.FS, path string) (string, error) {
	entries, err := fsys.ReadDir(path)
	if err != nil {
		return "", err
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	digests := make([]digestEntry, 0, len(entries))
	for _, entry := range entries {
		for _, previous := range digests {
			if strings.EqualFold(previous.Name, entry.Name()) {
				return "", invalid("case-insensitive source tree collision")
			}
		}
		hash, hashErr := HashTree(fsys, filepath.Join(path, entry.Name()))
		if hashErr != nil {
			return "", hashErr
		}
		digests = append(digests, digestEntry{Name: entry.Name(), Directory: entry.IsDir(), Hash: hash})
	}
	data, err := json.Marshal(digests)
	if err != nil {
		return "", err
	}
	return digest(append([]byte("kitbag-tree-v1\x00"), data...)), nil
}

func digest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
