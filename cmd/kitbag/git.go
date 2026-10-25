package main

import (
	"context"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded/sshauth"
	"github.com/TomaszJanusz/kitbag/internal/vcs/system"
)

// checkGitBackend fails when the user forced a backend that cannot be used,
// before any command runs, so a broken selection is never hidden by a
// command that happens not to need Git. Automatic selection is resolved
// lazily by the commands that use Git.
func checkGitBackend(ctx context.Context, sel vcs.Selection, host platform.Host) error {
	if !sel.Explicit() {
		return nil
	}
	_, _, err := vcs.Open(ctx, sel, gitFactories(host, vcs.Identity{}))
	return err
}

// gitFactories wires the two backends; identity is the embedded backend's
// fallback commit author from git.identity in config.toml.
func gitFactories(host platform.Host, identity vcs.Identity) vcs.Factories {
	return vcs.Factories{
		Embedded: func(context.Context) (vcs.Backend, error) {
			return embedded.New(embedded.Options{Env: host, Identity: identity, SSH: sshauth.Options{FS: host}}), nil
		},
		System: func(ctx context.Context) (vcs.Backend, error) {
			b, err := system.New(ctx, system.Options{Env: host})
			if err != nil {
				return nil, err
			}
			return b, nil
		},
	}
}
