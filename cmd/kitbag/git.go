package main

import (
	"context"
	"errors"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

const gitBackendFlag = "--git-backend"

var errNoValue = errors.New(gitBackendFlag + " needs a value: auto, system or embedded")

// globalFlags are the options every command accepts. The command tree of
// KIT-23 replaces this hand-written parsing with persistent flags.
type globalFlags struct {
	gitBackend string
}

// parseGlobalFlags removes --git-backend=VALUE and --git-backend VALUE from
// anywhere before "--" and returns the remaining arguments. A repeated flag
// keeps the last value.
func parseGlobalFlags(args []string) (globalFlags, []string, error) {
	var (
		flags globalFlags
		rest  = make([]string, 0, len(args))
	)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		value, isFlag := strings.CutPrefix(arg, gitBackendFlag)
		switch {
		case !isFlag:
			rest = append(rest, arg)
			continue
		case value == "" && i+1 < len(args):
			i++
			value = args[i]
		case value == "":
			return flags, nil, errNoValue
		case strings.HasPrefix(value, "="):
			value = value[1:]
		default:
			rest = append(rest, arg)
			continue
		}
		if value == "" {
			return flags, nil, errNoValue
		}
		flags.gitBackend = value
	}
	return flags, rest, nil
}

// selectGitBackend applies --git-backend > KITBAG_GIT_BACKEND > git.backend
// > auto. Reading git.backend from config.toml waits for the configuration
// loader (KIT-12/KIT-23).
func selectGitBackend(flags globalFlags, env platform.Env) (vcs.Selection, error) {
	fromEnv, _ := env.LookupEnv(vcs.EnvVar)
	return vcs.Select(flags.gitBackend, fromEnv, "")
}

// checkGitBackend fails when the user forced a backend that cannot be used,
// before any command runs, so a broken selection is never hidden by a
// command that happens not to need Git. Automatic selection is resolved
// lazily by the commands that use Git.
func checkGitBackend(ctx context.Context, sel vcs.Selection, env platform.Env) error {
	if !sel.Explicit() {
		return nil
	}
	_, _, err := vcs.Open(ctx, sel, gitFactories(env))
	return err
}

// gitFactories returns the backends this build provides; the embedded one is
// not implemented yet.
func gitFactories(env platform.Env) vcs.Factories {
	return vcs.Factories{
		System: func(ctx context.Context) (vcs.Backend, error) {
			b, err := system.New(ctx, system.Options{Env: env})
			if err != nil {
				return nil, err
			}
			return b, nil
		},
	}
}
