# Background synchronization

After `kitbag init`, install a per-user background service without root:

```sh
kitbag service install
kitbag service status
kitbag service status --json
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
identical install preserves the running job; changed definitions stop the old
job before atomic replacement. Switching modes removes obsolete unit files.
A failed supervisor operation returns an error; reinstall after correcting the
session or executable path. Configuration must already be set up with `init`.

Linux uses `dev.kitbag.agent.service` under `$XDG_CONFIG_HOME/systemd/user`
(default `~/.config/systemd/user`). Daemon units use `Type=simple`,
`Restart=on-failure`, `NoNewPrivileges=true`, and `WantedBy=default.target`.
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
Enablement uses `launchctl enable`; starting bootstraps the job (or kickstarts an
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
Global materialization/backend/host flags belong in machine configuration for
persistent operation; install's `--mode` selects daemon versus timer.

Status reports installed, loaded, active, enabled and stale definitions, the
supervisor's last run/exit code, and the existing per-repository last-sync record.
Unknown timestamps/codes stay empty/null; they do not claim a successful run.
For timers, active means scheduling is active, and run/exit information comes
from the oneshot service. Launchd does not expose a reliable wall-clock run
stamp through `print`; use `last_sync` for its synchronization timestamp.
JSON is versioned with `service_schema: 1`. Sync inspection failure is reported
separately and does not hide supervisor health. Doctor's `service` check is OK
when this optional feature is absent, and WARN when installed but inactive,
disabled, unloaded, stale, or the last supervisor exit code is nonzero. Neither status nor doctor changes supervisor state.

Uninstall stops/disables jobs, removes both systemd files or the plist and saved
installation inputs, then reloads systemd. It is safe to repeat. Configuration,
working copies, sync history and daemon logs are retained. No administrator or
other user's units are changed. Windows provides explicit unsupported stubs.
