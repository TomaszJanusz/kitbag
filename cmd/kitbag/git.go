package main

import (
	"context"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

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
