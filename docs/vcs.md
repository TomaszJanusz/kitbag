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
| `internal/vcs/embedded` | Basic Git in Go on go-git v5.19.3, without CGO or Git executables; also supplies the shared tree diff renderer. |

Lint confines `os/exec` to `internal/platform` and `internal/vcs/system`
(tests excepted): a child process inherits kitbag's whole environment, such
as `GIT_DIR`, unless the caller builds one. The system backend starts Git
through a `Runner` interface whose only real implementation, `ExecRunner`,
always passes an explicit environment; unit tests inject fakes.

## Operations

`Clone`, `Fetch`, `FastForward`, `Status` (branch, commit, upstream,
ahead/behind, local changes), `Diff` (name and kind of each path between two
revisions), `Preview` (unified text diff of committed trees), `Commit` (everything in the working tree), `Push`, `CreateBranch`,
`DeleteBranch`, `RemoteURL`, `RemoteHead` and `Head`. Every operation takes the absolute,
clean path of the working copy's top-level directory. The remote is always
`origin`. Errors are classified with `errors.Is`: `ErrNotFastForward`,
`ErrPushRejected`, `ErrNoUpstream`, `ErrDetachedHead`, `ErrNothingToCommit`,
`ErrNoRemote`, `ErrUnsupported` and `ErrInvalidArgument` (relative paths, revisions or names
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
level. `auto` uses system Git when installed and falls back to embedded only when
Git is missing from PATH. An installed Git that is too old, fails to start or
reports an unknown version remains an error; switching explicitly to embedded
is possible. A forced backend that cannot be used is an error;
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
`git 2.29.2 is older than the required 2.30` in the same place. Forcing `embedded` succeeds even with Git removed from PATH. A verbose sync
or status reports `embedded (go-git v5.19.3), selected by --git-backend` (or the
environment, file or default selection source).

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

## Embedded backend

The pinned stable v5 release is go-git **v5.19.3**. It includes the current v5
security fixes; v6 is still a prerelease. go-git imports are confined by depguard
to `internal/vcs/embedded`, including its tests. Builds need no CGO.

Full and shallow clones, fetch, clean fast-forward, status, unsigned commit,
push and basic branches work without a Git executable. The in-process file
transport also avoids `git-upload-pack` and `git-receive-pack`. Local shallow
clones transfer a full object pack and record shallow boundaries; HTTP/SSH
servers negotiate shallow transfers normally. Ahead/behind traversal respects
those boundaries. An empty local remote keeps its symbolic default branch; for
an empty network remote go-git cannot discover an unborn default branch, so an
explicit clone branch is preferred and otherwise `main` is used.

Authentication is deliberately limited to public HTTPS and SSH agent signing
through the platform's `SSH_AUTH_SOCK`. SSH verifies the real hostname against
`~/.ssh/known_hosts`; unknown or changed keys fail, without prompting. There is
no key-file search, HTTPS credential helper, token storage or interactive login
until credential discovery (M2). SSH aliases, identity files and general SSH
configuration are not applied; use an actual hostname and agent-loaded keys.

Embedded commits read identity from system Git configuration (unless disabled
for tests), XDG Git config, `~/.gitconfig`, then repository config, with later
values taking precedence. `git.identity.name` and `git.identity.email` in
machine `config.toml` supply missing values. No identity is invented; an
incomplete result fails before staging. These settings never change Git config.

Unsupported features return `*vcs.UnsupportedError`, matching `ErrUnsupported`,
with **"install git or resolve in the working copy"**. Non-fast-forward errors
also retain `ErrNotFastForward`.

"Detected" means the backend checks for the feature and returns the typed
error before changing anything; "documented only" means the limitation cannot
be seen in advance, so the operation may fail with an ordinary transport or
authentication error instead.

| Feature | Detected | Documented only | Behavior |
| --- | --- | --- | --- |
| Non-fast-forward merge | Divergence, from commit ancestry | — | `FastForward` fails without moving HEAD and also matches `ErrNotFastForward`; `Pull` reports `PullDiverged` and leaves the copy alone. |
| Rebase | `rebase-apply` / `rebase-merge` state, on commit and update | `pull.rebase` / `branch.*.rebase`: the interface never rebases, so they have no effect | No rebase API exists. |
| Merge drivers | Named `merge` configuration subsections, before commit, fetch, push and update | `merge=` attributes in `.gitattributes`; no merge ever runs | No merge driver is invoked. |
| rerere | Enabled `rerere.enabled` | — | No rerere state is recorded or applied. |
| Signed commits | Enabled `commit.gpgSign`, before staging | — | Embedded commits are unsigned. |
| SSH ProxyJump / ProxyCommand | Directives in `~/.ssh/config` or `/etc/ssh/ssh_config`, on SSH operations, even in an unrelated `Host` block | — | Conservative: the backend does not evaluate `Host` matching. |
| TLS client certificates | `http.sslCert` / `http.sslKey`, including URL subsections | Certificates required only by the server | The server's requirement cannot be inferred before connecting. |
| Kerberos | `http.emptyAuth` and SSH `GSSAPIAuthentication` | Negotiate/GSSAPI required only by the server | An ordinary authentication failure may be the only signal. |
| insteadOf rewrites | `url.*.insteadOf` and `pushInsteadOf`, before remote operations | — | URLs are used verbatim. |
| Git / SSH config includes | `include` / `includeIf` and SSH `Include` | — | Rejected rather than silently ignoring signing, credentials or unsupported settings behind them; included files are not read. |
| Hooks and advanced configuration | — | Git hooks, aliases, filters and general Git/SSH configuration | Never executed or applied; only the settings above are diagnosed. |

Go-git's checkout protections remain enabled. Its current worktree validator
rejects control characters in paths: locally inspecting/staging such names uses
a narrow rooted-filesystem compatibility path, but checkout/fast-forward of
remote trees containing them still fails safely. No checkout validation is
disabled. Normal spaces and Unicode work throughout.

## Shared previews

Both backends use the same go-git tree/blob comparison and unified patch renderer
(`object.Changes.PatchContext`, using the pinned transitive
`github.com/sergi/go-diff` **v1.3.2-0.20230802210424-5b0b94c5c0d3**). Renames are
reported as delete/add. Binary changes, file modes and missing final newlines
are rendered by this one implementation. Git attributes, textconv, external
diff programs and user diff settings do not influence previews. `Diff` retains
its existing path/kind API; `Preview` exposes the text patch. This repository
tree preview is separate from the CLI's existing logical target plan output.

## Working copy

`vcs.WorkingCopyDir(env, name)` is `<data>/repos/<name>`, where `<data>` is
the platform data directory (`~/.local/share/kitbag` by default, see
[platform.md](platform.md#directories)), and `vcs.RepoName(url)` derives the
name from the clone URL (`git@github.com:me/agent-config.git` →
`agent-config`). `repo.path` in `config.toml` overrides the location. The first `sync` clones an
absent working copy before applying its manifest; `--no-pull` requires an
existing copy. `init` remains separate work (KIT-26).

## Tests

`internal/vcs/contract_test.go` and its siblings hold one contract suite that
every backend passes unchanged (`TestSystemContract` and
`TestEmbeddedContract`). System Git creates the contract fixtures with a
temporary `HOME`, no system configuration and local bare repositories below
`t.TempDir()`: clone (including an empty remote), commit, push, status and diff
with awkward file names, pull (fast-forward, unborn, dirty, diverged),
non-fast-forward push rejection, branches, remote URL, detached HEAD, missing
upstream, unreachable and unreadable remotes, confinement to the working copy
and argument validation. Tests never assert on Git's human-readable messages,
which differ between versions. `cmd/kitbag/testdata/script/git_backend.txtar`
covers selection end to end, including Git removed from `PATH` (`hidecmd git`).
`sync_embedded.txtar` clones/applies and repeats silently with both forced
embedded and auto selection after hiding all three Git executables. Pure-Go
backend tests remove Git from PATH before even creating their fixtures and
exercise publishing, fetch, fast-forward, identity and preview. A parity test
compares both backends' exact preview bytes on the same repository.

## Remaining work

- Embedded credential discovery and broader SSH/configuration support (M2).
- Doctor capabilities for the embedded backend: it implements neither
  `vcs.RemoteProber` nor `vcs.CommitInspector`, so doctor reports those checks
  as skipped or unsupported (see [Doctor capabilities](#doctor-capabilities)).
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

## Doctor capabilities

`vcs.RemoteProber` optionally lists origin refs without fetching or updating
refs (`git ls-remote` in the system backend). `vcs.CommitInspector` exposes only
identity/signing configuration presence, not values; system Git implements it
by querying four named keys. Doctor bounds both operations with a context and
reports unsupported capabilities explicitly. Agent version probes reuse
`system.Runner` with a separate minimal environment; they never inherit agent
credentials or home/config variables. See [doctor.md](doctor.md).

`RemoteHead` advertises only the current branch's configured upstream commit
without downloading objects or changing local refs. It rejects detached HEAD,
missing upstream configuration and deleted refs with the existing sentinels.
The daemon uses it for [cheap polling](daemon.md). Both backends pass the shared
contract, including proof that the probe leaves cached ahead/behind counts alone.
