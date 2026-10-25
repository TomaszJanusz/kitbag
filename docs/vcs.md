# Git backends

Repository Git access goes through `vcs.Backend` in `internal/vcs`, so the embedded
backend can replace system Git without touching callers
([ADR 4](adr/0004-git-backends.md), plan §15.1 and §15.1a). This note
describes the boundary introduced by
[KIT-21](https://linear.app/privacybrand/issue/KIT-21); `go doc ./internal/vcs`
is the per-method reference.

## Packages

| Package | Role |
| --- | --- |
| `internal/vcs` | `Backend` interface, selection (`Select`, `Open`), the pull policy (`Pull`) and the working copy location. Starts no processes. |
| `internal/vcs/system` | `Backend` on the installed `git`, 2.30 or newer. |
| embedded (go-git) | Not implemented yet; selecting it is an error (see below). |

Lint confines `os/exec` to `internal/platform` and `internal/vcs/system`
(tests excepted): a child process inherits kitbag's whole environment, such
as `GIT_DIR`, unless the caller builds one. The system backend starts Git
through a `Runner` interface whose only real implementation, `ExecRunner`,
always passes an explicit environment; unit tests inject fakes.

## Operations

`Clone`, `Fetch`, `FastForward`, `Status` (branch, commit, upstream,
ahead/behind, local changes), `Diff` (name and kind of each path between two
revisions), `Commit` (everything in the working tree), `Push`, `CreateBranch`,
`DeleteBranch`, `RemoteURL` and `Head`. Every operation takes the absolute,
clean path of the working copy's top-level directory. The remote is always
`origin`. Errors are classified with `errors.Is`: `ErrNotFastForward`,
`ErrPushRejected`, `ErrNoUpstream`, `ErrDetachedHead`, `ErrNothingToCommit`,
`ErrNoRemote` and `ErrInvalidArgument` (relative paths, revisions or names
that look like options, empty messages).

Nothing in the interface merges, rebases, forces a push or discards changes.

## Pull policy

`vcs.Pull` is how sync updates its working copy:

| Working copy | Outcome | Fetches | Changes HEAD |
| --- | --- | --- | --- |
| Local changes, untracked files included | `PullSkippedDirty` | no | no |
| Nothing new upstream (possibly local commits ahead) | `PullUpToDate` | yes | no |
| Upstream ahead, no local commits | `PullUpdated` (fast-forward) | yes | yes |
| Local and upstream commits | `PullDiverged` | yes | no |
| Detached HEAD, or no upstream | error `ErrDetachedHead` / `ErrNoUpstream` | no | no |

Untracked files count as local changes because a fast-forward could
overwrite them. A branch that is still unborn (a clone of an empty remote) is
fast-forwarded once the remote has commits.

## Selection

The first of these that is set wins:

1. `--git-backend=auto|system|embedded` (also `--git-backend VALUE`), on every command;
2. the `KITBAG_GIT_BACKEND` environment variable;
3. `git.backend` in `config.toml`;
4. `auto`.

An empty value counts as unset. Only the winning value is parsed: an invalid
one is a usage error naming its source, never a reason to consult the next
level. `auto` uses system Git when a suitable version is installed and the
embedded backend otherwise. A forced backend that cannot be used is an error;
kitbag never falls back silently.

`vcs.Open` returns a `Resolution` with the requested backend, its source, the
active backend and its version, and in automatic mode why system Git was not
used. That is the hook for `doctor` (KIT-24), for example
`system (git 2.43.0), selected by --git-backend`.

When a backend is forced (by any of the three sources), every command checks
it before running, so a broken selection is reported immediately even by
commands that do not use Git. Automatic selection is resolved by the commands
that need Git.

### Messages

These messages are part of the interface; tests pin them. Forcing system Git
without `git` on `PATH` exits with status **1**:

```text
kitbag: Git backend "system" (selected by --git-backend) is unavailable: git was not found in PATH; install Git 2.30 or newer (https://git-scm.com/downloads)
```

The source reads `KITBAG_GIT_BACKEND` or `git.backend in config.toml` when
the selection came from there. A Git older than 2.30 reports
`git 2.29.2 is older than the required 2.30` in the same place. Until the
embedded backend exists, selecting it reports:

```text
kitbag: Git backend "embedded" (selected by --git-backend) is unavailable: the embedded Git backend is not available in this version of kitbag; use --git-backend=system and install Git 2.30 or newer (https://git-scm.com/downloads)
```

and `auto` without a usable Git, once commands use Git, names both reasons:
`Git backend "auto" (selected by default) is unavailable: system Git: git was
not found in PATH; embedded: the embedded Git backend is not available in this
version of kitbag; install Git 2.30 or newer (https://git-scm.com/downloads)`.

An unknown value exits with status **2**:

```text
kitbag: --git-backend: invalid Git backend "libgit2": want auto, system or embedded
```

## System backend

`system.New` looks for `git` in the `PATH` of the platform environment,
ignoring empty and relative entries, runs `git --version` and requires 2.30 or
newer. Vendor suffixes such as `(Apple Git-145)` or `.windows.1` are accepted.

Every Git process gets a complete, fixed environment rather than kitbag's:

| Variable | Value | Why |
| --- | --- | --- |
| `HOME` | `platform.Env.HomeDir()` | The user's `~/.gitconfig`, credentials and SSH files |
| `PATH`, `XDG_CONFIG_HOME`, `SSH_AUTH_SOCK`, `TMPDIR` | passed through when set | Helpers, XDG Git configuration, the SSH agent, macOS temporary files |
| `LC_ALL` | `C` | Untranslated output |
| `GIT_TERMINAL_PROMPT` | `0` | Fail instead of prompting for credentials |
| `GIT_EDITOR`, `GIT_SEQUENCE_EDITOR` | `false` | Never wait for an editor |
| `GIT_PAGER`, `PAGER` | `cat` | Never wait for a pager |
| `GIT_CEILING_DIRECTORIES` | parent of the working copy | A missing working copy never resolves to a repository above it |

No other variable is inherited, `GIT_*` included, so `GIT_DIR`,
`GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_CONFIG_*` or `GIT_SSH_COMMAND` in the
caller's environment cannot redirect or reconfigure kitbag's Git. The user's
configuration files still apply: credential helpers, `core.sshCommand`,
signing, hooks, `rerere` and merge drivers keep working (ADR 4). The system
configuration (`/etc/gitconfig`) is read too, because it may hold credential
helpers; tests disable it with `Options.NoSystemConfig`.

Output is read only in machine-readable forms: `status --porcelain=v2
--branch -z`, `diff --name-status -z`, `push --porcelain`, `symbolic-ref`,
`rev-parse` and `config --get`. Paths are NUL-separated and never quoted, so
names with spaces, newlines or any Unicode are exact. Options that user
configuration could change are fixed on the command line (`--ahead-behind`,
`--untracked-files=all`, `--no-renames`, `--no-relative`, `--no-autostash`,
`--origin=origin`, `--no-optional-locks` for status). User input is placed
after `--end-of-options` or `--` and rejected when it starts with `-`.

Not handled yet: SSH itself may still ask for a host key confirmation or key
passphrase on the controlling terminal; non-interactive SSH belongs to the
credential work.

## Working copy

`vcs.WorkingCopyDir(env, name)` is `<data>/repos/<name>`, where `<data>` is
the platform data directory (`~/.local/share/kitbag` by default, see
[platform.md](platform.md#directories)), and `vcs.RepoName(url)` derives the
name from the clone URL (`git@github.com:me/agent-config.git` →
`agent-config`). `repo.path` in `config.toml` overrides the location.

## Tests

`internal/vcs/contract_test.go` and its siblings hold one contract suite that
every backend must pass. It runs against the `git` on the test's `PATH` with a
temporary `HOME`, no system configuration and local bare repositories below
`t.TempDir()`: clone (including an empty remote), commit, push, status and diff
with awkward file names, pull (fast-forward, unborn, dirty, diverged),
non-fast-forward push rejection, branches, remote URL, detached HEAD, missing
upstream, unreachable and unreadable remotes, confinement to the working copy
and argument validation. Tests never assert on Git's human-readable messages,
which differ between versions. `cmd/kitbag/testdata/script/git_backend.txtar`
covers selection end to end, including Git removed from `PATH` (`hidecmd git`).

## Remaining work

- Embedded go-git backend and credential discovery (separate tickets); its
  factory plugs into `vcs.Factories` and its test file into the contract suite.
- Reading `git.backend` from `config.toml` (configuration loader, KIT-12/KIT-23).
- The cobra command tree and its persistent `--git-backend` flag (KIT-23);
  `doctor` output from `vcs.Resolution` (KIT-24).
- Windows: the environment pass-through list needs Windows variables
  (`USERPROFILE`, `APPDATA`, ...) with M6.

## Global Git includes

`vcs.GlobalConfig` is an optional system-backend capability for reading and
extending global `include.path` values. `vcs.EnsureIncludes` compares normalized
paths before checking write safety. [generic.md](generic.md) explains its
separate lifecycle and dotfile protections. Global commands alone honour an
absolute `GIT_CONFIG_GLOBAL`; repository commands still discard it and every
other unlisted `GIT_*` variable. The same Runner supplies the explicit clean
environment, adding only that override for global commands.
