# Background synchronization

KitBag is the CLI, the terminal UI and this per-user background service, which
needs no root. `kitbag init` and `kitbag adopt` offer to install it (the wizard's
default answer is yes; `--service` and `--no-service` decide without asking, see
[init.md](init.md#service-installation)). Install or inspect it at any time:

```sh
kitbag service install
kitbag service status
kitbag service status --json
kitbag service refresh --dry-run --json
kitbag service refresh
kitbag service uninstall
```

Daemon mode (the default) runs the [foreground daemon](daemon.md), including its
initial sync, remote polling, drift inspection and local API. Timer mode runs
plain `kitbag sync` on a schedule:

```sh
kitbag service install --mode timer --interval 15m
kitbag service install --no-start
kitbag service install --now
```

Installation enables and starts by default. `--now` makes that default explicit;
`--no-start` enables future starts without starting now. Intervals must be whole
seconds between 1 minute and 24 hours. They affect timer mode only. Repeating an
identical install preserves the running daemon only when its version also
matches this binary; changed definitions stop the old
job before atomic replacement. Switching modes removes obsolete unit files.
A failed supervisor operation returns an error; use refresh after correcting the
session or executable path. Configuration must already be set up with `init`.

Linux uses `dev.kitbag.agent.service` under `$XDG_CONFIG_HOME/systemd/user`
(default `~/.config/systemd/user`). Daemon units use `Type=simple`,
`Restart=on-failure`, `NoNewPrivileges=true`, and `WantedBy=default.target`.
`Restart=on-failure` also restarts the daemon after it exits with status 75
because its executable was replaced ([daemon.md](daemon.md#restart-after-an-upgrade)).
`PrivateTmp` is deliberately omitted: local Git remotes and agent socket paths
may live in `/tmp`. Timer mode adds `dev.kitbag.agent.timer`, with `OnBootSec=1min`,
`OnUnitActiveSec=<interval>`, `RandomizedDelaySec=30s` and `timers.target` enablement.
The initial timer run can therefore take up to 90 seconds after boot/activation.
Logs are available through `journalctl --user -u dev.kitbag.agent.service`;
the daemon also keeps its bounded logs in the platform log directory.

Installation checks `loginctl show-user $USER -p Linger`. With linger disabled,
the service works during the logged-in session. Headless operation, starting at
boot and continuing after logout need a persistent user manager. The command
prints `loginctl enable-linger <user>` for the current user; it never runs it.
An unavailable linger probe prints guidance without undoing installation.

macOS uses `~/Library/LaunchAgents/dev.kitbag.agent.plist`, label
`dev.kitbag.agent`, in `gui/<uid>`. The plist uses `RunAtLoad`, `ProcessType`
`Background`, and `KeepAlive` for daemon mode or `StartInterval` for timer mode.
`KeepAlive` restarts the daemon after any exit, including status 75 after a
replaced executable. Enablement uses `launchctl enable`; starting bootstraps the job (or kickstarts an
already loaded job). `--no-start` writes/enables the job for the next login.
Uninstall boots it out, disables it, and removes the plist. Standard output and
error go to `service.stdout.log` and `service.stderr.log` in the platform log
directory. These supervisor streams are separate from the bounded daemon logs.
Real launchd installation still requires maintainer QA on a Mac.

The resolved absolute running executable and absolute configuration path are
written into definitions; paths with spaces are quoted without a shell. Units
are atomic 0644 files in a private directory. Saved installation inputs are 0600
in `<state>/service.json`. Only HOME, PATH, XDG paths and supported agent home
path overrides are persisted. API keys, tokens, the SSH agent environment and
other variables are not copied. Keep service credentials in the user's normal
Git credential configuration. Restart/reinstall when those path inputs change.

On Linux, PATH is the installing shell's PATH. On macOS the plist's PATH holds
only what the daemon runs: the directories in which the installing shell's PATH
finds `git` and `ssh` (in that order, links not resolved), followed by launchd's
default `/usr/bin:/bin:/usr/sbin:/sbin`. Programs that Git itself starts from
other directories, such as credential helpers or `git-lfs`, need an absolute
path in the Git configuration, or must live next to `git`. Reinstall after
installing Git or OpenSSH somewhere else.

### Homebrew

Homebrew runs the binary from a versioned keg such as
`/opt/homebrew/Cellar/kitbag/0.1.0/libexec/kitbag`, which `brew upgrade` plus cleanup
deletes. When the resolved executable lies in a keg, the plist uses the stable
`<prefix>/opt/<formula>/libexec/kitbag` link instead (or `<prefix>/bin/kitbag` when
only that exists; the formula’s `bin/kitbag` links to `libexec/kitbag`), but
only if the link currently resolves to the running
binary; otherwise the keg path is kept. Other installations keep the resolved
path. The same stable-link selection now applies to Homebrew on Linux, so future
upgrades are watched through opt rather than a disappearing keg path. Refresh
repairs previously installed Linux keg paths.

### Refresh after upgrades

`kitbag service refresh [--json] [--dry-run]` repairs an **existing** installation.
It uses the same layout and renderer as install, retaining saved configuration,
mode, timer interval, log directory and allow-listed environment. The current
binary path and generator policy (including macOS PATH narrowing) replace the
old ones. Comments, blank lines and outer directive whitespace in systemd files,
and XML formatting/comments/dictionary order in plists, do not cause a refresh.
Argument contents and directive/array order remain significant. Local changes
to generated directives are replaced; put persistent settings in machine config.

Refresh writes each definition atomically before changing the supervisor. It
reloads systemd and restarts an active unit when the definition changed or the
running daemon version differs. On launchd, a loaded definition is removed with
bootout and loaded from the new plist with bootstrap; kickstart alone cannot
replace it. Refresh preserves enablement. An unloaded launchd job or inactive systemd
unit stays unstarted (including installations using `--no-start`). A pending record retains incomplete reload/restart work, so retrying
a failed refresh can finish even when the files already match. Missing/corrupt
saved inputs cannot safely reconstruct a custom installation: the command fails
and explains that `service install` is needed to select its inputs again.

An uninstalled service is a successful no-op. Matching definitions and versions
also do no supervisor work. Both install and refresh inspect the running daemon,
even when a stable Homebrew plist/unit path is unchanged. Older daemons, absent
version fields and unavailable version handshakes on an active supervised daemon
are treated as **older**, rather than as proof the process is current. An actual
version transition prints `Restarted the running daemon (0.1.1 -> 0.1.2)`;
unknown versions print `older` in place of the old version. Install only says
“already installed and up to date” for a running daemon whose version matches.
`install --no-start` explicitly defers a needed restart and says so.
Authentication and incompatible-protocol errors still fail rather than being
misreported as version matches. `--dry-run` performs only observations, reports the
proposed rewrite/restart and writes nothing (including no locks or markers).
JSON retains `service_schema: 1` and adds `installed`, `definition_changed`,
`restart_needed`, `changed`, `dry_run`, `binary_version` and optional
`daemon_version`. Exit **0** means success, including no-op/dry-run; **1** means
an operation failed; **2** means invalid input; **5** means unsupported OS.
A dry run that finds work still exits 0. Concurrent refreshes report busy rather
than overlap. Supervisor failures remain errors; retry `service refresh` after
correcting the session/path. Ordinary status, doctor, plan and sync never refresh
or restart a service. The TUI offers a confirmed **h** repair action.

A new daemon automatically checks its own installed daemon definition at startup;
see [startup repair](daemon.md#startup-service-repair). Version 0.1.2 and newer
also restart after executable replacement. A running **0.1.1 or older daemon**
cannot gain that behavior without first running the new binary: use refresh or
the TUI repair action once after upgrading it. No package hook is installed to
restart that older process silently. Timer runs pick up a new binary on their
next start. `brew uninstall kitbag` removes the executable link; refresh cannot
restore an uninstalled binary.

### Why there is no Homebrew post-install hook

Homebrew source and documentation inspected on 2026-10-26 at Homebrew/brew
commit `c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc`:

- New Ruby `post_install` methods are rejected by formula audits; formulae use
  the restricted `post_install_steps` DSL instead, which runs "with the same
  post-install sandbox policy"
  ([Formula Cookbook](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/docs/Formula-Cookbook.md#L1092)).
- [FormulaInstaller#post_install](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/formula_installer.rb#L1509-L1526)
  runs the steps in `Sandbox`, granting writes only to the formula's logs, keg
  and link directories, and applies
  [`add_install_hook_rules`](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/sandbox.rb#L613-L618):
  temporary/cache writes, [denied home reads](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/sandbox.rb#L429)
  and no network unless the formula allows it.
- The [macOS profile](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/extend/os/mac/sandbox.rb#L21-L68)
  denies Unix socket connections (line 24), every non-allow-listed write
  (line 38) and Mach lookups (line 41). `kitbag service refresh` must read the
  user's configuration, write `~/Library/LaunchAgents` and its state directory,
  ask the daemon's socket for its version and talk to launchd: all outside those
  grants. A `run "kitbag", args: ["service", "refresh"]` step therefore cannot do
  its job on macOS.
- On Linux the sandbox is [Landlock](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/extend/os/linux/sandbox.rb#L44)
  when the kernel supports it, so post-install steps are not unrestricted either.
  Even without confinement, a package installation does not identify which
  user's `systemd --user` manager owns the KitBag service.
- The DSL's [`terminate_process`](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/docs/Formula-Cookbook.md#L1174) step runs
  [`pkill -f` or `killall`](https://github.com/Homebrew/brew/blob/c6069d2a943b71b0a90d91fc1ac93cb9c1634dcc/Library/Homebrew/install_steps.rb#L1291-L1300)
  (SIGTERM). This is the only step that could reach an old daemon. On macOS the
  0.1.1 plist sets `KeepAlive` to true, so launchd would start the stable `opt`
  path again. On Linux the 0.1.1 unit uses `Restart=on-failure`, and systemd
  treats SIGTERM as a clean exit that does not restart
  ([systemd.service](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html#SuccessExitStatus=)),
  so the hook would stop the service. It would also kill foreground daemons and
  interrupt a running sync, and the Seatbelt profile's effect on signals has not
  been tested on a real Mac.

These are source-based findings, not a sandbox or real-supervisor experiment.
The hook cannot be shown to be safe, so the formula is unchanged and the
snapshot formula job has no post-install step. Daemons from 0.1.2 onwards
restart themselves after an upgrade and repair their own definition at startup;
for an older daemon, status, doctor and the TUI warn and offer an explicit repair.

Global materialization/backend/host flags belong in machine configuration for
persistent operation; install's `--mode` selects daemon versus timer.

Status reports installed, loaded, active, enabled and stale definitions, the
supervisor's last run/exit code, and the existing per-repository last-sync record.
Unknown timestamps/codes stay empty/null; they do not claim a successful run.
Launchd prints `last exit code = (never exited)` for a job that has not exited
since it was loaded, the normal state of a healthy daemon; status reports it as
an unknown (null) exit code. JSON `binary` is the executable recorded at
installation and `binary_missing` is true when it no longer exists.
For timers, active means scheduling is active, and run/exit information comes
from the oneshot service. Launchd does not expose a reliable wall-clock run
stamp through `print`; use `last_sync` for its synchronization timestamp.
JSON is versioned with `service_schema: 1`. Sync inspection failure is reported
separately and does not hide supervisor health. Doctor's `service` check
WARNs when the service is not installed (this machine then syncs only on
`kitbag sync`), and when installed but inactive,
disabled, unloaded, stale, its executable is missing, or the last supervisor
exit code is nonzero other than the daemon's restart status 75. Neither status nor doctor changes supervisor state.

Uninstall stops/disables jobs, removes both systemd files or the plist and saved
installation inputs, then reloads systemd. It is safe to repeat. Configuration,
working copies, sync history and daemon logs are retained. No administrator or
other user's units are changed. Windows provides explicit unsupported stubs.

## Last check and last sync

The daemon checks the remote every `sync.poll_interval` (60 seconds by default),
with bounded jitter and failure backoff. `Last check` records every completed
check, including no change and failures; `Last sync` records attempts to apply
changes. An old sync can be healthy when checks are recent. The initial sync
also records a check. Failed checks show a fixed error-catalog category, never
raw Git output, credentials or remote URLs. `Next check` is the actual next
scheduled time, including jitter/backoff. It follows completed work.

`kitbag status` uses local time with relative age in text, and UTC RFC 3339 in
JSON. When no daemon runs, the persisted last check is explicitly stale and
`Next check` is hidden. `kitbag doctor` warns when an installed, active daemon's
persisted check is older than three configured poll intervals. It reads one
small local record and performs no extra remote request. Timer services are
excluded. Failure backoff or a long sync can also exceed this threshold; inspect
the service logs before restarting.
