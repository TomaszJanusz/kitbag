package vcs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// GlobalConfig reads and extends the user's global Git configuration, the
// files "git config --global" uses.
type GlobalConfig interface {
	// GlobalConfigFiles lists the files Git would write, in Git's order: the
	// GIT_CONFIG_GLOBAL file alone when set, else ~/.gitconfig and then the
	// XDG file. Git writes the first that exists, else the first.
	GlobalConfigFiles() []string
	// GlobalIncludes returns every include.path value of the global
	// configuration, verbatim and without following includes.
	GlobalIncludes(ctx context.Context) ([]string, error)
	// AddGlobalInclude appends one include.path value.
	AddGlobalInclude(ctx context.Context, path string) error
}

// StatFS is the read-only file access [EnsureIncludes] needs; a
// platform.FS satisfies it.
type StatFS interface {
	Lstat(path string) (fs.FileInfo, error)
}

// IncludeResult reports what [EnsureIncludes] did.
type IncludeResult struct {
	// Added lists the include paths written, in request order.
	Added []string
	// Warnings explain includes that were missing but not written.
	Warnings []string
}

// EnsureIncludes adds each absolute path in paths as an include.path of the
// global Git configuration unless an equal entry already exists. Existing
// values are compared after expanding a leading "~" to home and cleaning, so
// "~/x" equals "<home>/x". Entries are never removed or duplicated. When a
// path is missing and the file Git would write is a symbolic link (for example
// managed by chezmoi or another dotfile tool), not a regular file, or
// read-only, nothing is written and a warning names the include to add there.
func EnsureIncludes(ctx context.Context, cfg GlobalConfig, fsys StatFS, home string, paths []string) (IncludeResult, error) {
	var result IncludeResult
	if err := validateIncludes(home, paths); err != nil {
		return result, err
	}
	existing, err := cfg.GlobalIncludes(ctx)
	if err != nil {
		return result, err
	}
	missing := missingIncludes(existing, home, paths)
	if len(missing) == 0 {
		return result, nil
	}
	file, refusal, err := writableConfig(fsys, cfg.GlobalConfigFiles())
	if err != nil {
		return result, err
	}
	if refusal != "" {
		for _, path := range missing {
			result.Warnings = append(result.Warnings, fmt.Sprintf("global Git config %s %s; add include.path %s there", file, refusal, path))
		}
		return result, nil
	}
	for _, path := range missing {
		if err := cfg.AddGlobalInclude(ctx, path); err != nil {
			return result, err
		}
		result.Added = append(result.Added, path)
	}
	return result, nil
}

// normalizeInclude expands "~" and "~/..." like Git does for include.path.
func normalizeInclude(value, home string) string {
	switch {
	case value == "~":
		return home
	case strings.HasPrefix(value, "~/"):
		return filepath.Join(home, filepath.FromSlash(value[2:]))
	}
	return filepath.Clean(value)
}

// writableConfig picks the file Git would write and explains why kitbag must
// not let Git rewrite it, if so.
func writableConfig(fsys StatFS, candidates []string) (file, refusal string, err error) {
	if len(candidates) == 0 {
		return "", "", errors.New("no global Git configuration file")
	}
	for _, candidate := range candidates {
		info, err := fsys.Lstat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", "", fmt.Errorf("global Git config: %w", err)
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			return candidate, "is a symbolic link (managed elsewhere)", nil
		case !info.Mode().IsRegular():
			return candidate, "is not a regular file", nil
		case info.Mode().Perm()&0o200 == 0:
			return candidate, "is read-only", nil
		}
		return candidate, "", nil
	}
	return candidates[0], "", nil
}

func validateIncludes(home string, paths []string) error {
	for _, path := range append([]string{home}, paths...) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("git include %q is not an absolute clean path: %w", path, ErrInvalidArgument)
		}
	}
	return nil
}

func missingIncludes(existing []string, home string, paths []string) []string {
	present := make([]string, 0, len(existing))
	for _, value := range existing {
		present = append(present, normalizeInclude(value, home))
	}
	missing := make([]string, 0, len(paths))
	for _, path := range paths {
		if !slices.Contains(present, path) && !slices.Contains(missing, path) {
			missing = append(missing, path)
		}
	}
	return missing
}
