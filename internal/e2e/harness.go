// Package e2e is the end-to-end test harness: it runs testscript scripts
// against a throwaway HOME inside each script's $WORK and isolated Git
// configuration, and provides commands to arrange agent directories and Git
// remotes and to assert symlink targets, file content and silent runs.
//
// The package is test-only: import it from _test.go files (enforced by depguard).
package e2e

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rogpeppe/go-internal/testscript"
)

// gitConfig is the only global Git configuration scripts see. System
// configuration is disabled and hooks never run.
const gitConfig = `[init]
	defaultBranch = main
[commit]
	gpgSign = false
[tag]
	gpgSign = false
[core]
	hooksPath = /dev/null
	autocrlf = false
[advice]
	detachedHead = false
`

// Params returns testscript parameters that run every script in dir with
// [Setup] and [Commands]. Commands registered with testscript.Main must be
// started with exec.
func Params(dir string) testscript.Params {
	return testscript.Params{
		Dir:                 dir,
		Setup:               Setup,
		Cmds:                Commands(),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
	}
}

// realHomesKey holds, in a script's values, the real home directories that
// the harness's commands must refuse.
type realHomesKey struct{}

// Setup gives a script a throwaway HOME at $WORK/home, pre-populated by any
// home/ sections of the script, and an isolated Git environment. It refuses to
// continue if that HOME would be a real home directory.
func Setup(env *testscript.Env) error {
	return setup(env, RealHomes())
}

func setup(env *testscript.Env, realHomes []string) error {
	home := filepath.Join(env.WorkDir, "home")
	if err := CheckHome(home, env.WorkDir, realHomes); err != nil {
		return err
	}
	env.Values[realHomesKey{}] = realHomes
	if err := os.MkdirAll(home, privateDir); err != nil {
		return fmt.Errorf("create HOME: %w", err)
	}
	configPath := filepath.Join(env.WorkDir, ".gitconfig")
	if err := os.WriteFile(configPath, []byte(gitConfig), privateFile); err != nil {
		return fmt.Errorf("write Git configuration: %w", err)
	}

	env.Setenv("HOME", home)
	for _, kv := range [][2]string{
		{"GIT_CONFIG_GLOBAL", configPath},
		{"GIT_CONFIG_NOSYSTEM", "1"},
		{"GIT_AUTHOR_NAME", "KitBag Test"},
		{"GIT_AUTHOR_EMAIL", "test@kitbag.invalid"},
		{"GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z"},
		{"GIT_COMMITTER_NAME", "KitBag Test"},
		{"GIT_COMMITTER_EMAIL", "test@kitbag.invalid"},
		{"GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z"},
		// Only local file remotes: scripts never reach the network.
		{"GIT_ALLOW_PROTOCOL", "file"},
		{"GIT_TERMINAL_PROMPT", "0"},
		// Untranslated messages keep stderr assertions stable.
		{"LC_ALL", "C"},
	} {
		env.Setenv(kv[0], kv[1])
	}
	return nil
}
