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

// passThrough lists the only variables of kitbag's environment that Git sees,
// besides HOME: PATH finds Git's helpers (credential helpers, ssh, editors of
// hooks), XDG_CONFIG_HOME locates the user's Git configuration, SSH_AUTH_SOCK
// reaches the SSH agent and TMPDIR is the per-user temporary directory on
// macOS. In particular no GIT_* variable is inherited, so GIT_DIR,
// GIT_WORK_TREE, GIT_INDEX_FILE and the like cannot redirect kitbag.
func passThrough() []string {
	return []string{"PATH", "XDG_CONFIG_HOME", "SSH_AUTH_SOCK", "TMPDIR"}
}

// Options configure [New].
type Options struct {
	// Env supplies PATH, HOME and the other pass-through variables.
	Env platform.Env
	// Runner starts Git; nil selects [ExecRunner].
	Runner Runner
	// NoSystemConfig ignores the system-wide Git configuration
	// (GIT_CONFIG_NOSYSTEM). Production keeps it: it may configure credential
	// helpers. Tests set it for isolation.
	NoSystemConfig bool
}

// Backend is the vcs.Backend that runs the git executable. Create it with
// [New].
type Backend struct {
	runner  Runner
	git     string
	env     []string
	version Version
}

var _ vcs.Backend = (*Backend)(nil)

// New finds git on the PATH of opts.Env and checks that it is at least
// version 2.30. It fails with [ErrGitNotFound], a [*VersionError] (matching
// [ErrGitTooOld]) or [ErrUnknownVersion].
func New(ctx context.Context, opts Options) (*Backend, error) {
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	env, err := environment(opts)
	if err != nil {
		return nil, err
	}
	pathList, _ := opts.Env.LookupEnv("PATH")
	git, err := runner.LookPath("git", pathList)
	if err != nil {
		// The runner only reports "not found"; the sentinel is the message.
		return nil, ErrGitNotFound
	}
	b := &Backend{runner: runner, git: git, env: env}

	res, err := b.run(ctx, rootOf(git), "--version")
	if err != nil {
		return nil, err
	}
	if b.version, err = ParseVersion(string(res.Stdout)); err != nil {
		return nil, err
	}
	if err := checkVersion(b.version); err != nil {
		return nil, err
	}
	return b, nil
}

// environment builds the complete, fixed environment of every Git process.
func environment(opts Options) ([]string, error) {
	if opts.Env == nil {
		return nil, fmt.Errorf("system backend: no environment: %w", vcs.ErrInvalidArgument)
	}
	home, err := opts.Env.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("system backend: %w", err)
	}
	env := []string{"HOME=" + home}
	for _, key := range passThrough() {
		if value, ok := opts.Env.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	env = append(env,
		// Untranslated, machine-readable output.
		"LC_ALL=C",
		// Fail instead of waiting for input nobody can give.
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=false",
		"GIT_SEQUENCE_EDITOR=false",
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
	if opts.NoSystemConfig {
		env = append(env, "GIT_CONFIG_NOSYSTEM=1")
	}
	return env, nil
}

// Kind returns vcs.System.
func (*Backend) Kind() vcs.Kind {
	return vcs.System
}

// Version returns "git" and the version Git reported.
func (b *Backend) Version() string {
	return "git " + b.version.String()
}

// Path returns the absolute path of the git executable in use.
func (b *Backend) Path() string {
	return b.git
}

// Clone runs git clone. The remote is always named origin, whatever
// clone.defaultRemoteName says.
func (b *Backend) Clone(ctx context.Context, url, dir string, opts vcs.CloneOptions) error {
	if err := checkDir(dir); err != nil {
		return err
	}
	if url == "" {
		return fmt.Errorf("clone: empty URL: %w", vcs.ErrInvalidArgument)
	}
	args := []string{"clone", "--quiet", "--origin=" + vcs.DefaultRemote}
	if opts.Branch != "" {
		args = append(args, "--branch="+opts.Branch)
	}
	// Run outside any repository: dir may not exist yet.
	_, err := b.run(ctx, rootOf(dir), append(args, "--", url, dir)...)
	return err
}

// Fetch runs git fetch --prune on the default remote.
func (b *Backend) Fetch(ctx context.Context, dir string) error {
	_, err := b.repo(ctx, dir, "fetch", "--quiet", "--prune", "--end-of-options", vcs.DefaultRemote)
	return err
}

// FastForward runs git merge --ff-only with the upstream branch after
// checking that it is a fast-forward.
func (b *Backend) FastForward(ctx context.Context, dir string) error {
	s, err := b.Status(ctx, dir)
	if err != nil {
		return err
	}
	switch {
	case s.Detached():
		return fmt.Errorf("fast-forward: %w", vcs.ErrDetachedHead)
	case s.Upstream == "":
		return fmt.Errorf("fast-forward %s: %w", s.Branch, vcs.ErrNoUpstream)
	case s.UpstreamGone:
		return fmt.Errorf("fast-forward %s: upstream %s is gone: %w", s.Branch, s.Upstream, vcs.ErrNoUpstream)
	case s.Behind == 0:
		return nil
	case s.Ahead > 0:
		return fmt.Errorf("fast-forward %s to %s: %w", s.Branch, s.Upstream, vcs.ErrNotFastForward)
	}
	// --no-autostash: a user's merge.autoStash must not stash and re-apply
	// local changes behind kitbag's back.
	_, err = b.repo(ctx, dir, "merge", "--ff-only", "--quiet", "--no-autostash", "--no-edit", "@{upstream}")
	return err
}

// Status runs git status --porcelain=v2.
func (b *Backend) Status(ctx context.Context, dir string) (vcs.Status, error) {
	res, err := b.repo(ctx, dir, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "-z",
		"--untracked-files=all", "--ahead-behind")
	if err != nil {
		return vcs.Status{}, err
	}
	s, err := parseStatus(res.Stdout)
	if err != nil {
		return vcs.Status{}, err
	}
	if s.Unborn() && s.Upstream != "" {
		// Git reports no ahead/behind counts without a local commit; every
		// upstream commit is new.
		err = b.unbornBehind(ctx, dir, &s)
	}
	return s, err
}

func (b *Backend) unbornBehind(ctx context.Context, dir string, s *vcs.Status) error {
	res, err := b.exec(ctx, dir, "rev-list", "--count", "@{upstream}")
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		s.UpstreamGone = true
		return nil
	}
	_, err = fmt.Sscan(string(res.Stdout), &s.Behind)
	if err != nil {
		return fmt.Errorf("rev-list --count: %w", err)
	}
	return nil
}

// Diff runs git diff --name-status between two revisions.
func (b *Backend) Diff(ctx context.Context, dir, from, to string) ([]vcs.Change, error) {
	if err := checkRevisions(from, to); err != nil {
		return nil, err
	}
	res, err := b.repo(ctx, dir, "diff", "--name-status", "-z", "--no-renames", "--no-relative",
		"--no-color", "--no-ext-diff", "--no-textconv", "--end-of-options", from, to, "--")
	if err != nil {
		return nil, err
	}
	return parseDiff(res.Stdout)
}

// Commit stages everything with git add --all and runs git commit. The
// user's hooks and signing configuration apply.
func (b *Backend) Commit(ctx context.Context, dir string, opts vcs.CommitOptions) (string, error) {
	if strings.TrimSpace(opts.Message) == "" {
		return "", fmt.Errorf("commit: empty message: %w", vcs.ErrInvalidArgument)
	}
	if _, err := b.repo(ctx, dir, "add", "--all"); err != nil {
		return "", err
	}
	res, err := b.exec(ctx, dir, "diff", "--cached", "--quiet", "--no-ext-diff")
	switch {
	case err != nil:
		return "", err
	case res.ExitCode == 0:
		return "", fmt.Errorf("commit: %w", vcs.ErrNothingToCommit)
	case res.ExitCode != 1:
		return "", b.commandError([]string{"diff", "--cached"}, res)
	}
	if _, err = b.repo(ctx, dir, "commit", "--quiet", "--message="+opts.Message); err != nil {
		return "", err
	}
	h, err := b.Head(ctx, dir)
	return h.Commit, err
}

// Push runs git push --porcelain with an explicit refspec, never forcing.
func (b *Backend) Push(ctx context.Context, dir string, opts vcs.PushOptions) error {
	branch := opts.Branch
	if branch == "" {
		h, err := b.Head(ctx, dir)
		if err != nil {
			return err
		}
		if h.Detached() {
			return fmt.Errorf("push: %w", vcs.ErrDetachedHead)
		}
		branch = h.Branch
	}
	if err := checkName("branch", branch); err != nil {
		return err
	}
	args := []string{"push", "--porcelain"}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	ref := "refs/heads/" + branch
	args = append(args, "--end-of-options", vcs.DefaultRemote, ref+":"+ref)
	res, err := b.exec(ctx, dir, args...)
	if err != nil {
		return err
	}
	if reason, rejected := pushRejection(res.Stdout); rejected {
		return fmt.Errorf("push %s: %w: %s", branch, vcs.ErrPushRejected, reason)
	}
	if res.ExitCode != 0 {
		return b.commandError(args, res)
	}
	return nil
}

// CreateBranch runs git branch --no-track.
func (b *Backend) CreateBranch(ctx context.Context, dir, name, start string) error {
	if start == "" {
		start = "HEAD"
	}
	if err := checkName("branch", name); err != nil {
		return err
	}
	if err := checkRevisions(start); err != nil {
		return err
	}
	_, err := b.repo(ctx, dir, "branch", "--no-track", "--end-of-options", name, start)
	return err
}

// DeleteBranch runs git branch --delete, which refuses unmerged branches.
func (b *Backend) DeleteBranch(ctx context.Context, dir, name string) error {
	if err := checkName("branch", name); err != nil {
		return err
	}
	_, err := b.repo(ctx, dir, "branch", "--delete", "--end-of-options", name)
	return err
}

// RemoteURL reads remote.<name>.url from the repository configuration.
func (b *Backend) RemoteURL(ctx context.Context, dir, remote string) (string, error) {
	if err := checkName("remote", remote); err != nil {
		return "", err
	}
	args := []string{"config", "--get", "remote." + remote + ".url"}
	res, err := b.exec(ctx, dir, args...)
	switch {
	case err != nil:
		return "", err
	case res.ExitCode == 1:
		return "", fmt.Errorf("remote %s: %w", remote, vcs.ErrNoRemote)
	case res.ExitCode != 0:
		return "", b.commandError(args, res)
	}
	return strings.TrimSuffix(string(res.Stdout), "\n"), nil
}

// Head reads HEAD with git symbolic-ref and git rev-parse.
func (b *Backend) Head(ctx context.Context, dir string) (vcs.Head, error) {
	var h vcs.Head
	ref, err := b.optional(ctx, dir, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return h, err
	}
	h.Branch = strings.TrimPrefix(ref, "refs/heads/")
	h.Commit, err = b.optional(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return h, err
}

// optional runs a query whose exit status 1 means "no value".
func (b *Backend) optional(ctx context.Context, dir string, args ...string) (string, error) {
	res, err := b.exec(ctx, dir, args...)
	switch {
	case err != nil:
		return "", err
	case res.ExitCode == 1:
		return "", nil
	case res.ExitCode != 0:
		return "", b.commandError(args, res)
	}
	return string(bytes.TrimSuffix(res.Stdout, []byte("\n"))), nil
}

// repo runs Git in the working copy dir and turns a failure into an error.
func (b *Backend) repo(ctx context.Context, dir string, args ...string) (Result, error) {
	res, err := b.exec(ctx, dir, args...)
	if err == nil && res.ExitCode != 0 {
		err = b.commandError(args, res)
	}
	return res, err
}

// run runs Git with working directory cwd, outside a repository.
func (b *Backend) run(ctx context.Context, cwd string, args ...string) (Result, error) {
	res, err := b.runner.Run(ctx, Command{Path: b.git, Args: args, Dir: cwd, Env: b.env})
	if err == nil && res.ExitCode != 0 {
		err = b.commandError(args, res)
	}
	return res, err
}

// exec runs Git in the working copy dir. GIT_CEILING_DIRECTORIES stops Git
// from using a repository above dir when dir itself is not one.
func (b *Backend) exec(ctx context.Context, dir string, args ...string) (Result, error) {
	if err := checkDir(dir); err != nil {
		return Result{}, err
	}
	env := slices.Concat(b.env, []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir)})
	return b.runner.Run(ctx, Command{Path: b.git, Args: args, Dir: dir, Env: env})
}

func (b *Backend) commandError(args []string, res Result) error {
	return &CommandError{Args: args, ExitCode: res.ExitCode, Stderr: string(bytes.TrimSpace(res.Stderr))}
}

// CommandError reports a Git command that exited with a non-zero status.
type CommandError struct {
	// Stderr is Git's diagnostic output, trimmed.
	Stderr   string
	Args     []string
	ExitCode int
}

// Error names the subcommand, the exit status and Git's message.
func (e *CommandError) Error() string {
	name := "git"
	if len(e.Args) > 0 {
		name += " " + e.Args[0]
	}
	if e.Stderr == "" {
		return fmt.Sprintf("%s: exit status %d", name, e.ExitCode)
	}
	return fmt.Sprintf("%s: exit status %d: %s", name, e.ExitCode, e.Stderr)
}

// rootOf returns the root directory of path's volume, a working directory
// that exists and is not inside any repository's influence on the result.
func rootOf(path string) string {
	return filepath.VolumeName(path) + string(filepath.Separator)
}

func checkDir(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return fmt.Errorf("working copy %q is not an absolute clean path: %w", dir, vcs.ErrInvalidArgument)
	}
	return nil
}

func checkRevisions(revs ...string) error {
	for _, rev := range revs {
		if rev == "" || strings.HasPrefix(rev, "-") {
			return fmt.Errorf("revision %q: %w", rev, vcs.ErrInvalidArgument)
		}
	}
	return nil
}

func checkName(what, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, " \t\n\x00") {
		return fmt.Errorf("%s name %q: %w", what, name, vcs.ErrInvalidArgument)
	}
	return nil
}
