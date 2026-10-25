package system

import (
	"context"
	"errors"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/vcs"
)

// ProbeRemote lists origin's refs without updating the local repository.
func (b *Backend) ProbeRemote(ctx context.Context, dir string) error {
	_, err := b.repo(ctx, dir, "ls-remote", "--quiet", "--", vcs.DefaultRemote)
	return err
}

// InspectCommit reads only named effective Git configuration keys. Values never
// leave this backend, and no signing program or credential helper is invoked.
func (b *Backend) InspectCommit(ctx context.Context, dir string) (vcs.CommitConfig, error) {
	var cfg vcs.CommitConfig
	name, err := b.optional(ctx, dir, "config", "--get", "user.name")
	if err != nil {
		return cfg, err
	}
	email, err := b.optional(ctx, dir, "config", "--get", "user.email")
	if err != nil {
		return cfg, err
	}
	signing, err := b.optional(ctx, dir, "config", "--bool", "--get", "commit.gpgsign")
	if err != nil {
		return cfg, err
	}
	key, err := b.optional(ctx, dir, "config", "--get", "user.signingkey")
	cfg.Identity = strings.TrimSpace(name) != "" && strings.TrimSpace(email) != ""
	cfg.Signing, cfg.SigningKey = signing == "true", strings.TrimSpace(key) != ""
	return cfg, err
}

// ProbeVersion runs a known agent's --version with a minimal environment and
// the supplied runner. It never passes credentials or reads agent configuration.
func ProbeVersion(ctx context.Context, env platform.Env, runner Runner, name string) (string, error) {
	pathList, _ := env.LookupEnv("PATH")
	path, err := runner.LookPath(name, pathList)
	if err != nil {
		return "", err
	}
	res, err := runner.Run(ctx, Command{Path: path, Args: []string{"--version"}, Dir: rootOf(path), Env: []string{"PATH=" + pathList, "LC_ALL=C"}})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 || len(res.Stdout) == 0 {
		return "", errors.New("version probe failed")
	}
	return strings.Join(strings.Fields(string(res.Stdout)), " "), nil
}
