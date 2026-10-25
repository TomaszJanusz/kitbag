package system

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

var _ vcs.GlobalConfig = (*Backend)(nil)

// globalConfig locates the user's global Git configuration. Repository
// commands deliberately ignore GIT_CONFIG_GLOBAL (see passThrough); global
// configuration commands honour it, so kitbag edits the file the user's own
// Git reads.
//
// A relative or unclean config path would make safety checks ambiguous; it
// fails only the global configuration commands, never repository commands.
type globalConfig struct {
	err   error
	env   []string
	files []string
}

// newGlobalConfig follows Git: GIT_CONFIG_GLOBAL replaces both files; an
// empty XDG_CONFIG_HOME means ~/.config. env already holds HOME.
func newGlobalConfig(source platform.Env, home string, env []string) globalConfig {
	if file, ok := source.LookupEnv("GIT_CONFIG_GLOBAL"); ok && file != "" {
		if !filepath.IsAbs(file) || filepath.Clean(file) != file {
			return globalConfig{err: fmt.Errorf("GIT_CONFIG_GLOBAL %q is not absolute and clean: %w", file, vcs.ErrInvalidArgument)}
		}
		return globalConfig{
			env:   append(slices.Clone(env), "GIT_CONFIG_GLOBAL="+file),
			files: []string{filepath.Clean(file)},
		}
	}
	xdg, ok := source.LookupEnv("XDG_CONFIG_HOME")
	if !ok || xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	if !filepath.IsAbs(xdg) || filepath.Clean(xdg) != xdg {
		return globalConfig{err: fmt.Errorf("XDG_CONFIG_HOME %q is not absolute and clean: %w", xdg, vcs.ErrInvalidArgument)}
	}
	return globalConfig{
		env:   env,
		files: []string{filepath.Join(home, ".gitconfig"), filepath.Join(xdg, "git", "config")},
	}
}

// homeOf returns HOME from an environment built by environment.
func homeOf(env []string) string {
	for _, kv := range env {
		if home, ok := strings.CutPrefix(kv, "HOME="); ok {
			return home
		}
	}
	return ""
}

// GlobalConfigFiles lists the candidate global configuration files; it is
// empty when GIT_CONFIG_GLOBAL is unusable.
func (b *Backend) GlobalConfigFiles() []string {
	return slices.Clone(b.globalConfig.files)
}

func (b *Backend) global(ctx context.Context, args ...string) (Result, error) {
	if b.globalConfig.err != nil {
		return Result{}, b.globalConfig.err
	}
	return b.runner.Run(ctx, Command{Path: b.git, Args: args, Dir: rootOf(b.git), Env: b.globalConfig.env})
}

// GlobalIncludes runs git config --global --get-all include.path. Includes
// are not followed: only the global files' own entries count.
func (b *Backend) GlobalIncludes(ctx context.Context) ([]string, error) {
	args := []string{"config", "--global", "--no-includes", "--null", "--get-all", "include.path"}
	res, err := b.global(ctx, args...)
	switch {
	case err != nil:
		return nil, err
	case res.ExitCode == 1 && len(res.Stdout) == 0:
		// No such key (or no global file yet).
		return nil, nil
	case res.ExitCode != 0:
		return nil, b.commandError(args, res)
	}
	values := bytes.Split(bytes.TrimSuffix(res.Stdout, []byte{0}), []byte{0})
	includes := make([]string, 0, len(values))
	for _, value := range values {
		includes = append(includes, string(value))
	}
	return includes, nil
}

// AddGlobalInclude runs git config --global --add include.path path.
func (b *Backend) AddGlobalInclude(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("include path %q is not an absolute clean path: %w", path, vcs.ErrInvalidArgument)
	}
	args := []string{"config", "--global", "--add", "include.path", path}
	res, err := b.global(ctx, args...)
	if err == nil && res.ExitCode != 0 {
		err = b.commandError(args, res)
	}
	return err
}
