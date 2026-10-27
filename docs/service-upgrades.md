# Service upgrade investigation

The maintainer observed a version mismatch after a Homebrew upgrade from 0.1.2
to 0.1.3. This investigation distinguishes source evidence and fake-host checks
from launchd runtime evidence. No live Mac was available; the cause on that Mac
cannot be confirmed without its loaded job definition and logs.

## Evidence

- In [v0.1.2 daemon.go](https://github.com/TomaszJanusz/kitbag/blob/v0.1.2/cmd/kitbag/daemon.go),
  `daemonExecutableCheck` is one minute, and `watchExecutable` captures the
  service layout's binary path. In
  [v0.1.2 executable.go](https://github.com/TomaszJanusz/kitbag/blob/v0.1.2/internal/daemonhost/executable.go),
  `Replaced` needs two settled metadata observations and a different digest.
  Detection can therefore take roughly 60–120 seconds, plus active work and
  supervisor scheduling when the watched path remains usable. The supplied
  roughly 130-second Linux observation is consistent with that delay, but
  timing alone does not prove recovery: the
  [rehearsal script](../scripts/upgrade-rehearsal.sh) also uses 130 seconds as
  its observation window for cases expected not to self-heal. It was not rerun
  here. In
  [v0.1.2 service_linux.go](https://github.com/TomaszJanusz/kitbag/blob/v0.1.2/internal/platform/unix/service_linux.go)
  the binary remains versioned, whereas
  [v0.1.2 service_darwin.go](https://github.com/TomaszJanusz/kitbag/blob/v0.1.2/internal/platform/unix/service_darwin.go)
  already calls `stableExecutable`. Linux keg-cleanup behavior is therefore
  not evidence that the Mac takes the same path.
- [v0.1.2 generate.go](https://github.com/TomaszJanusz/kitbag/blob/v0.1.2/internal/core/service/generate.go)
  emits unconditional `KeepAlive=true`. Exit 75 is a nonzero restart signal,
  not a launchd-specific magic status. Apple's
  [launchd.plist source manual](https://github.com/apple-oss-distributions/launchd/blob/main/man/launchd.plist.5)
  describes unconditional KeepAlive, conditional SuccessfulExit, and a default
  ten-second spawn throttle. SuccessfulExit=true conditions restart on a zero
  exit, while false conditions it on failure; that dictionary is not emitted
  here. No custom ThrottleInterval is emitted. Throttle delays spawning; it
  does not permanently suppress restart. This is
  source-based reasoning, not verification on the maintainer's macOS release.
- [unix/service.go](../internal/platform/unix/service.go), `stableExecutable`,
  recognizes both `bin/kitbag` and `libexec/kitbag` in versioned Cellar kegs and
  selects an opt/bin link only when it currently resolves to the running
  executable. [homebrew-formula.sh](../scripts/homebrew-formula.sh) installs to
  libexec and creates the bin link. A stable service path follows link rotation;
  a legacy loaded definition naming the deleted old keg cannot relaunch it.
- [unix/fs.go](../internal/platform/unix/fs.go), `Stat`, follows symlinks.
  [daemonhost/executable.go](../internal/daemonhost/executable.go) uses that
  target's size, mtime and mode, then a content digest; it does not compare link
  text. The watcher keeps its startup path after the old keg is deleted.
  The new fake-filesystem regression rotates a stable link, deletes its old
  target, accepts an identical-content replacement without restart, and detects
  changed content. This proves the requested policy through the fake FS, not
  native launchd behavior. Unchanged size/mtime/mode can still conceal changed
  content from the identity optimization; this change does not claim otherwise.

## Changes

The new daemon checks identity every five seconds, retaining two observations
and digest comparison. This shortens future passive detection to roughly
5–10 seconds before active-work/supervisor delays. It does not change code
already running in a 0.1.2 daemon.

New installed status/doctor/TUI clients automatically use
[service.RecoverVersion](../internal/core/service/recovery.go), outside the
process being stopped. Recovery reuses Refresh's saved-input rewrite and
systemd reload/restart or launchd bootout/bootstrap sequence. It acts only on
its own saved config, enabled daemon mode, a usable installed executable and a
newer observed client version (or an older daemon without version metadata).
Matching versions, older clients, stopped/disabled jobs and timers are excluded.
A loaded launchd job whose saved Cellar path is missing can also be repaired.

A durable marker and refresh lock limit attempts to one per binary version per
ten minutes, including failures. The pending notice lasts at most 150 seconds;
failed attempts and persistent mismatches expose diagnostics rather than a
permanent “upgrading” state. Authorization/protocol failures do not authorize
recovery. Normal upgrades require no user-side refresh command.

## Limits and validation

Core/fake-host tests prove policy guards, desired supervisor calls, rewriting
of a removed saved Cellar path, cooldown across new clients, the bounded notice,
and visible failures. Doctor, adapter and shared-model tests prove automatic
invocation and pending versus persistent presentation. TUI goldens cover both states.
Temporary mutations prove the new regressions detect broken guards.

Not verified here: native launchd restart, throttle and process-group behavior;
a live Homebrew upgrade on macOS; Linux systemd upgrade timing in this run;
Windows runtime. Existing Darwin source/stub tests and Linux tests do not prove
launchd runtime. A stale legacy job with no running new client cannot acquire
new repair code by itself; that migration case still needs a live Mac rehearsal
before claiming unattended recovery of every prior installation.
