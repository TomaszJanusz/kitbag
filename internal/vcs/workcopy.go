package vcs

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// reposDir is the directory below the platform data directory that holds
// working copies (docs/platform.md).
const reposDir = "repos"

// WorkingCopyDir returns the default working copy location for the
// repository called name: <data>/repos/<name>, where <data> is kitbag's
// platform data directory. name must be a single path component, such as the
// result of [RepoName].
func WorkingCopyDir(env platform.Env, name string) (string, error) {
	if !validRepoName(name) {
		return "", fmt.Errorf("working copy name %q: %w", name, ErrInvalidArgument)
	}
	dirs, err := env.Dirs()
	if err != nil {
		return "", fmt.Errorf("working copy: %w", err)
	}
	return filepath.Join(dirs.Data, reposDir, name), nil
}

// RepoName derives a working copy name from a clone URL: the last path
// component without a ".git" suffix, so "git@github.com:me/agent-config.git"
// becomes "agent-config".
func RepoName(url string) (string, error) {
	trimmed := strings.TrimRight(url, "/")
	if i := strings.LastIndexAny(trimmed, `/:\`); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	name := strings.TrimSuffix(trimmed, ".git")
	if !validRepoName(name) {
		return "", fmt.Errorf("repository name of %q: %w", url, ErrInvalidArgument)
	}
	return name, nil
}

// validRepoName accepts one non-hidden path component that is the same on
// every operating system.
func validRepoName(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".") &&
		!strings.ContainsAny(name, `/\:`) && filepath.IsLocal(name)
}
