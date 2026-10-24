# Platform layer

The core never learns which operating system it runs on. Every host operation
goes through the interfaces in `internal/platform`, as required by
[ADR 3](adr/0003-materialization-and-platform.md) and plan §18.1. This note
describes the contract introduced by
[KIT-13](https://linear.app/privacybrand/issue/KIT-13/platform-layer-interface-unix-implementation-in-memory-fake-windows)
and the decisions behind it. The package documentation (`go doc
./internal/platform`) is the authoritative per-method reference.

## Packages

| Package | Builds on | Role |
| --- | --- | --- |
| `internal/platform` | all | `Host` = `FS` + `Env`, error sentinels, `CheckOS` |
| `internal/platform/unix` | macOS, Linux | The real v1 host |
| `internal/platform/windows` | Windows | Stubs returning `ErrUnsupported` until M6 |
| `internal/platform/fake` | all | In-memory host for core tests |

Lint enforces the boundaries: `syscall` and `golang.org/x/sys` stay in
`internal/platform`; environment and user-directory lookups (`os.Getenv`,
`os.LookupEnv`, `os.Environ`, `os.UserHomeDir`, `os.UserConfigDir`, ...) stay
in `internal/platform`; `os/exec` stays in `internal/platform` and the system
Git backend ([vcs.md](vcs.md)), which pass child processes an explicit
environment; only `cmd/` may import `unix` or `windows`; only tests
may import `fake`; and `internal/core`, tests included, may not import `os`,
so it cannot touch the real filesystem.

## Interface

`FS` (materialization):

| Method | Semantics | Main consumers |
| --- | --- | --- |
| `Lstat`, `Stat` | Describe an entry without or with following a final link | plan, adopt |
| `ReadFile`, `ReadDir` | Read through links; `ReadDir` sorts by name | plan (hashes), state, adopt |
| `ReadLink` | Return a link target verbatim | adopt, doctor |
| `IsManagedLink(path, roots...)` | Link whose target lies within a root, compared lexically by whole component; dangling links count | apply (repair), adopt |
| `MkdirAll(path, perm)` | Create missing directories with exactly `perm` | apply, state |
| `WriteAtomic(path, data, perm)` | Atomically replace a file or link with a file of exactly `perm` | apply (copy mode), state |
| `Link(target, path)` | Atomically replace a file or link with a symbolic link | apply (link mode) |
| `Rename(old, new)` | Move an entry; `new` must not exist | backups, adopt `--move-real-dirs` |
| `Remove(path)` | Delete a file, a link (never its target) or an empty directory | apply, unmanage |
| `Chmod(path, perm)` | Set exactly `perm`; refuses links | state, hooks |
| `TryLock(path)` | Non-blocking exclusive lock | state store |

`Env` (environment): `LookupEnv`, `HomeDir`, `Dirs` (kitbag's config, data,
state and cache directories), `AgentHome(override, rel)`, `Hostname`,
`Open(target)` and `IsTerminal(fd)`.

Not in this layer yet: services and keyring (M2), the runtime and log
directories needed by the daemon socket and LaunchAgent (KIT-28, KIT-29), and
Windows filesystem naming constraints, which manifest validation checks
independently of the OS (KIT-12). Adding a method means adding it to every
implementation and to the contract suite.

## Semantics

**Paths.** Every path argument is absolute and clean in host syntax. Relative
or unclean paths fail with `fs.ErrInvalid` before the host is touched, so the
process working directory never matters and `..` is never resolved lexically
where the kernel would resolve it physically. Links in intermediate components
are always followed.

**Replacement.** `WriteAtomic` and `Link` replace an existing file or link in a
single rename, never a directory (`ErrIsDir`), and never write through a link:
writing over a link to the working copy replaces the link and leaves the
repository file untouched. The parent directory must already exist. Whether an
existing entry may be replaced (an unmanaged file needs a backup first, for
example) is core policy, not platform behavior.

On macOS, replacing a link has one caveat observed in CI: a program reading
*through* the old link at the moment of the rename can get a transient
`EINVAL` from `open(2)`, although the directory entry is never missing and
never anything but the old or the new link. Linux readers always see one of
the two targets. The contract suite asserts the stronger property where it
holds and tolerates only that error on macOS. Only a read that races the
rename is affected; a later read sees the new link. Regular files replaced by
`WriteAtomic` showed no such effect.

**Case.** Default macOS volumes are case-insensitive; Linux and the fake are
case-sensitive. The platform does not normalize names, so callers must never
rely on two names that differ only in case (manifest validation rejects such
collisions independently of the OS, plan §18.1).

**Durability (unix).** `WriteAtomic` writes a `.kitbag-*.tmp` sibling, sets the
mode, syncs the file (Go uses `F_FULLFSYNC` on macOS), renames it over the
destination and syncs the directory. `Link`, `Rename` and `Remove` also sync the
directories they change. A failure before the rename removes the temporary
entry and leaves the old content; a crash can leave a `.kitbag-*.tmp` file but
never a truncated destination. A failed directory sync after the rename is
reported although the new content is already in place. Filesystems that cannot
sync directories (`EINVAL`, `ENOTSUP`) are tolerated.

**Modes.** `WriteAtomic`, `MkdirAll` and `Chmod` apply exactly the requested
permission bits regardless of the umask, so the result is the same on every
machine and in the fake. Only permission bits are accepted (`fs.ErrInvalid`
otherwise). `MkdirAll` leaves existing directories unchanged. Replacing a file
does not preserve its old mode; a caller that wants to preserve it passes the
mode it read with `Lstat`.

**Rename.** `Rename` refuses an existing destination (`fs.ErrExist`), so a
backup never overwrites an earlier one. The existence check and the rename are
two steps; kitbag serializes its own writers with `TryLock`, and no other
program is expected to create kitbag's backup names. A rename across
filesystems fails with `ErrCrossDevice`.

**Locks.** `TryLock` uses `flock(2)`. The kernel releases the lock when the
process exits, so a crash never leaves a stale lock; locks conflict within one
process too. The lock file is created with mode 0600 and left in place.
Network filesystems may not honor `flock`; the state directory is local.

**Errors.** Errors match `fs.ErrNotExist`, `fs.ErrExist`, `fs.ErrPermission`
or `fs.ErrInvalid` where those apply, otherwise `ErrIsDir`, `ErrNotDir`,
`ErrNotEmpty`, `ErrNotLink`, `ErrCrossDevice`, `ErrLocked` or `ErrUnsupported`
(the standard library's `errors.ErrUnsupported`). The unix host keeps the
native message and errno while adding the sentinel, so callers classify with
`errors.Is` only.

**Open.** `Open` accepts an absolute path or an `http`/`https` URL with a host,
nothing else, so manifest content cannot launch arbitrary protocol handlers and
no target can be mistaken for an option. The unix host runs `open` (macOS) or
`xdg-open` (Linux) without a shell.

## Directories

macOS and Linux use the same XDG Base Directory layout:

| `Dirs` field | Default | Override | Holds |
| --- | --- | --- | --- |
| `Config` | `~/.config/kitbag` | `$XDG_CONFIG_HOME/kitbag` | `config.toml` |
| `Data` | `~/.local/share/kitbag` | `$XDG_DATA_HOME/kitbag` | working copies (`repos/<name>`) |
| `State` | `~/.local/state/kitbag` | `$XDG_STATE_HOME/kitbag` | `state.json`, history, lock |
| `Cache` | `~/.cache/kitbag` | `$XDG_CACHE_HOME/kitbag` | rebuildable data (catalog index) |

As the XDG specification requires, an empty or relative `XDG_*` value is
ignored. `HOME` must be set and absolute.

### Why macOS does not use `~/Library/Application Support`

The ADRs leave this open; the decision follows the plan and the agent
inventory:

- The plan names `~/.config/kitbag/config.toml`, `~/.local/share/kitbag/repos`
  and XDG state for Unix (§2.2, §3, §17.1), pairing them only with Windows'
  `%APPDATA%` and `%LOCALAPPDATA%` (§18.1).
- kitbag is terminal-first. The macOS conventions in the
  [agent inventory](agents.md) split the same way: CLI agents such as Goose and
  Devin's MCP configuration use `~/.config` on macOS and Linux alike, while
  `~/Library/Application Support` appears for GUI applications (VS Code) and
  system-wide managed policy (`/Library/Application Support/...`).
- One layout means one documented path per file, the same dotfile management
  on both machines (KIT-43) and the same test isolation (`HOME` plus `XDG_*`
  variables) everywhere; the contract suite verifies one code path. It also
  avoids a space in every path used in shell snippets and unit files.
- Users who relocate XDG directories on macOS get the behavior they expect.

Consequences: `os.UserConfigDir` and `os.UserCacheDir`, which return Library
paths on macOS, are not used and are blocked by lint outside the platform
layer. macOS does not purge `~/.cache` the way it may purge `~/Library/Caches`;
the cache holds only small, rebuildable data. Service files keep their native
locations (`~/Library/LaunchAgents`, KIT-28): those are service management,
not kitbag's data. Moving to Library paths later would need a migration, so a
change should be a new decision record.

### Agent homes

`AgentHome(override, rel)` returns the absolute directory named by the
environment variable `override` (for example `CODEX_HOME`) when it is set and
non-empty, otherwise `rel` (slash-separated, below the home directory). Unlike
XDG variables, a relative override is an error rather than ignored: the agent
itself honors the variable, so silently falling back would write where the
agent does not read. Variables with other semantics, such as `GEMINI_CLI_HOME`
naming the parent of `.gemini` ([inventory item 5](agents.md#manifest-implications-for-kit-4)),
are composed by adapters from `LookupEnv` and `HomeDir`.

## Windows before 1.x

Windows compiles from M1 (`mise run check:windows`), but compilation is not a
claim of support. `platform.CheckOS("windows")` reports an
`UnsupportedOSError`, and `cmd/kitbag` prints

```text
kitbag: Windows is not supported before 1.x
```

to standard error and exits with code **5**, for every command. The code
follows the exit codes planned for the apply engine (0 success, 1 warnings,
2 usage, 3 conflict, 4 tool too old, KIT-16). The `windows` package returns
`ErrUnsupported` wrapped with the operation name from every method; M6
(KIT-66) replaces it without changes outside `internal/platform`.

## State store

The machine-local store and its lock/history contracts are documented in
[state.md](state.md). Core state tests use the fake; one process integration
test under `cmd/kitbag` verifies the Unix kernel lock across process boundaries.

## Contract tests

`internal/platform/contract_test.go` and its sibling files hold one suite
that runs against `unix` (real `t.TempDir` directories and an injected
environment; never the real `HOME`, never a real opener) and against `fake`.
Each case runs in parallel on a fresh subject. Two cases depend on host
capabilities and skip when they are unavailable:

- Permission denial needs a non-root user, because root bypasses mode bits.
- Cross-device renames need a second filesystem: `/dev/shm` on Linux, a
  simulated mount in the fake; macOS runners have none.

One case declares a host property instead of skipping: reading through a link
during its replacement may fail with `fs.ErrInvalid` on macOS (see
Semantics).

White-box tests in `internal/platform/unix` cover what the contract cannot
observe: the sync-before-rename order, cleanup after injected failures, the
opener command and terminal detection on a pseudo-terminal (the master on
Linux; the slave on macOS, whose master is not a terminal). A new
implementation (M6) adds a factory next to `contract_unix_test.go` and must
pass the same suite.
