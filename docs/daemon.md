# Foreground daemon

`kitbag daemon` keeps a configured machine current and serves the authenticated
[local API](api/local-api.md). Run `kitbag init` first, then run the daemon in the
foreground or under a user service supervisor. The command does not install a
service; use [service management](service.md) to install one. Linux and macOS are supported; Windows remains a compiling stub.

```sh
kitbag daemon
kitbag status --json
```

The initial pass uses the same pull, plan, recovery and apply workflow as
`kitbag sync`. Subsequent passes advertise the current branch's configured
upstream through `vcs.Backend.RemoteHead`: system Git uses `ls-remote --refs`;
embedded Git lists advertised refs without fetching objects. The probe changes
neither local refs nor the working copy. A changed remote commit triggers sync.
Unchanged remote heads perform no fetch or apply. GitHub ETag polling of the
configuration repository is a future optimization; ordinary Git remotes work now.

The existing `sync.poll_interval` setting defaults to **60s**, bounded from
**1s to 24h**. Polls use 90–100% of the interval to spread machines' requests.
The next timer starts after the previous operation finishes; network and apply
time add latency. A probe has a 15s deadline. Failures double the delay up to
15 minutes (or the configured interval when larger), with the same jitter.
Success restores the normal interval. Dirty/diverged copies retain normal sync
safety: no automatic commits, rebases, forced replacement or discard. A remote
head that was not applied is retried with backoff. Missing-clone startup failures
retry the normal sync bootstrap.

One scheduler executes its probe, sync, inspection and update check serially.
API and CLI syncs also share the existing nonblocking state lock; a competing
operation reports busy instead of overlapping. SIGINT/SIGTERM cancel network/API
work, close clients and wait for active operations. An apply already writing
finishes its journaled steps before shutdown; the apply recovery guarantees also
cover abrupt process death. The daemon does not introduce another state format.

## Drift watching

fsnotify watches the working copy and declared destinations, including their
parents so atomic replacement is detected. Working-copy manual edits and target
drift publish `drift.detected` after a read-only inspection. Watch hints **never
trigger apply**. The next explicit sync, or a moved remote head, uses the normal
three-way drift protection. Editing through a managed link changes the working
copy and is reported as working-copy dirtiness.

`daemon.debounce` defaults to **500ms**, bounded from **100ms to 1m**. The first
hint opens a fixed coalescing window; continuous writes cannot postpone inspection
indefinitely. A one-element queue coalesces hints without blocking the native
collector. At most 512 directories, eight levels below each root, and 4096
entries are traversed per watch rebuild. Symlinks and `.git` subdirectories are
not traversed. Missing destinations watch their nearest existing parent.
Directory creation and replacement refresh the watch set after inspection.

A full read-only inspection runs every minute even when native watching is
unavailable, overflows, reaches a resource/recursion limit, or misses an event.
Watcher failures are logged and leave periodic inspection active. They do not
turn a successfully applied commit into a failed sync. Large repositories may
therefore have up to one-minute drift-detection latency. Filesystem scans can
cost more than the small idle fixture measured below.

## Lifetime, API and logs

The platform state directory holds `daemon.lock`. Its kernel lock is held for
the entire lifetime, independent of the shorter sync lock. A second instance
reports `daemon already running for this user`. Kernel release on process exit
makes lock recovery independent of PID reuse. Only after acquiring this lock
does startup consider a stale API socket. It verifies the endpoint's private
permissions, ownership and connection refusal before removing it. Live servers,
regular files, symlinks and unsafe endpoints are refused. Normal exit removes
the owned socket and releases locks.

The API emits `sync.started`, `sync.finished` (including exit code and changed
count), `drift.detected`, and `update.available`. Existing bounded API subscriber
queues drop hints when full, so a slow client cannot block synchronization.
Clients refresh snapshots after reconnecting; events are not a durable journal.

`Dirs.LogDir` is `<state>/log`, following `XDG_STATE_HOME` on both supported
systems. `daemon.jsonl` contains UTC timestamps, action names and failure booleans;
it excludes request data, file contents, Git diagnostics and credentials. Files
are 0600 in a 0700 directory. A small in-repository rotator keeps the current file
at most 1 MiB plus three numbered archives (at most 4 MiB total). Files with
modification times older than seven days are removed on the next log write;
active logs remain size-bounded. Writes are atomic; rotation/log failures stop
the daemon with an operational error. No log goroutine or rotation dependency
is retained at idle.

## Restart after an upgrade

The daemon notices when its own executable file is replaced, for example by
`brew upgrade`, a package manager, `install.sh` or `kitbag self-update`, and
restarts onto the new version without user action. It watches the executable
path that `kitbag service install` writes into the service definition
([service.md](service.md)): the resolved running executable, or on macOS the
stable Homebrew link. The path is followed through links, so a link that moves to
a new version counts.

Every minute it compares the file's size, modification time and mode with the
file it started from. A changed identity must be seen unchanged by two
consecutive checks, so a file still being written is not used, and its SHA-256
content digest (streamed, not held in memory) must differ from the starting
file. Then the scheduler finishes the active operation, the daemon releases its
locks and socket like a normal shutdown, logs `restart`, and exits with status
**75** (`EX_TEMPFAIL`). The service manager starts the new binary: systemd
units use `Restart=on-failure`, which restarts after any nonzero status, and the
LaunchAgent uses `KeepAlive`. A daemon run by hand in a terminal just exits;
start it again.

It never loops. Identical content (a reinstall of the same version) becomes the
new baseline without a restart. A missing, unreadable or non-executable file is
logged as a failed `executable` check every minute and the daemon keeps running
the old version, because a restart could not start anything. A restarted daemon
records its own baseline, so one replacement restarts it once. Only an actual
new binary triggers the next restart.

A Linux Homebrew installation, whose unit names the versioned keg path, and a
macOS service installed by 0.1.0 still name an executable that `brew cleanup`
deletes. The daemon then keeps running, and `kitbag service status` and
`doctor` report the missing executable; run `kitbag service install` once.
Status and doctor report exit status 75 as a restart, not a failure: launchd
keeps showing it as the last exit code while the new daemon runs.

## Release metadata

`daemon.update_check` defaults to **true**. The daemon checks GitHub's latest
release metadata at startup, then daily. It caches the endpoint, ETag, tag and
successful-check timestamp as private `release-check.json` in the state directory.
Restarting within a day uses cached metadata. Subsequent requests send
`If-None-Match`; 304 reuses the cached tag. Semantic versions newer than the running
version publish `update.available`. This is a notification only: the checker
never downloads assets, follows redirects, installs, or restarts the tool; only
an executable that is already replaced restarts the daemon (above).

The shared `internal/releasehttp` transport serves metadata-only requests for
the daemon, independently of its explicit self-update asset-fetch API. The
response is limited to 64 KiB with a 10s deadline. Invalid metadata, draft or
prerelease responses and HTTP failures are logged and retried the next day.
`daemon.release_url` selects a compatible latest-release JSON endpoint (default
`https://api.github.com/repos/TomaszJanusz/kitbag/releases/latest`). HTTP is
supported on loopback for local testing; other endpoints require HTTPS. Disable network update
checks with `daemon.update_check = false`. Restart after changing daemon scheduling
settings. Workflow/API operations reload repository and materialization settings
for each call; token rotation also requires restart.

## Memory

The idle budget is **under 20 MB RSS** (`VmRSS`) with the default 60s interval.
Most of it is the binary itself: on Linux about 13.5 MiB is clean file-backed
text and read-only data paged in at startup. Package initialization touches
pages across the whole binary, and the kernel maps neighbouring cached pages
too. The Go heap holds about 1 MiB live while idle. Each inspection allocates
transient planner and validation data that is not retained between cycles.

The daemon command therefore tunes only its own process. It caps `GOMAXPROCS`
at **2**, because the scheduler is serial and two Ps serve it and the API.
It sets `GOGC=50` to bound heap growth within a cycle. An explicit `GOMAXPROCS`
or `GOGC` environment variable wins. After timer-driven work (initial sync,
poll, periodic scan, update check), the scheduler calls `debug.FreeOSMemory`
and returns free heap pages to the OS. That is at most a few forced
collections per poll interval of a roughly 1 MiB heap, never one per watch
hint. Debounced watch scans rely on the next periodic trim.

## Reproducible idle measurement

On Linux, the following builds the production binary and runs it against a
throwaway HOME and local bare remote. It disables update checks, uses system Git
and the default 60s polling interval, waits for initial apply, and samples `/proc`
once per second. It pushes one upstream change mid-run (at 120s, or half of a
shorter run) and reports whether it was applied. It records start/peak/end/mean
RSS, the anonymous/file-backed split at the end, process CPU time, and CPU
including Git children. Startup/build cost is outside the timed window. The
tool sends SIGTERM and removes only its own temporary directory on exit.

```sh
mise exec -- bash scripts/daemon-bench.sh 2
mise exec -- bash scripts/daemon-bench.sh 10
```

The acceptance measurement uses a realistic machine instead. Run it as an
unprivileged account with a real HOME, outside `mise`, so `GOMAXPROCS` is unset:

1. Build with `go build -ldflags "-X main.version=0.1.0" ./cmd/kitbag`.
2. Create the configuration repository as that account: a local bare remote of
   an agent-configuration tree (12 skills, about 40 files). Then run
   `kitbag init --yes <remote>`, `adopt` and `sync`. Init defaults apply:
   automatic Git backend, links and the daily release check, which contacts
   GitHub at startup.
3. Start `kitbag daemon`. Sample `VmRSS`, `RssAnon` and `RssFile` from
   `/proc/<pid>/status` every 10s for 600s. Push one upstream change at 120s.
4. Confirm the change was applied, record user+system CPU from
   `/proc/<pid>/stat`, then stop the daemon with SIGTERM and check exit 0.

Results on runner02 (Linux, 8 vCPU) on 2026-10-25, back to back on an otherwise
quiet host, 60 samples per run:

| Build | Start RSS | Max RSS | End RSS | CPU in 600s |
| --- | --- | --- | --- | --- |
| Before (`634b2fc`) | 21,568 KiB | 22,580 KiB | 22,516 KiB | 0.93s |
| After (`230c597`) | 18,572 KiB | 19,604 KiB | 19,228 KiB | 1.01s |

Anonymous RSS fell from 8.0–8.9 MiB to 5.0–5.9 MiB; about 13.7 MiB stayed
file-backed. The upstream change was applied, and SIGTERM exited 0. Peak margin
under the budget is about 0.4 MB; peaks occur during a cycle. macOS RSS,
production-sized repositories, authenticated network remotes and uptimes beyond
ten minutes are **not verified**.

`TestDaemonIdleHeap` is the automated regression guard. It re-executes the test
binary, runs production daemon wiring in that isolated process against a local
remote with 1 MiB of managed files, and applies three remote changes. It then
requires live heap growth (minimum of several samples after two collections) to
stay under 1 MiB, and `HeapInuse` under 8 MiB. It also checks that the tuning,
the explicit-environment override and the idle trim are in effect. It asserts
no RSS or timing; `-short` skips it.

The fast test suite covers local-remote push/application, process exclusion,
SIGTERM and context shutdown, restart after a replaced executable (and none for
identical or missing files), stale sockets, target drift preservation, scheduler
backoff/debounce/serialization, watch limits, metadata-only HTTP/ETag behavior,
and log size/count/age bounds. These tests run in `mise run check` on Linux/macOS.

Temporary negative controls were run and reverted: concurrent scheduler syncs,
allowing another daemon instance, downloading release assets, accepting a zero
interval, force-applying from a watcher, and blocking event publication. Each
was rejected by its focused test (the zero-interval and blocking-publication
controls intentionally reached the test timeout). The drift check runs before
any update creates a one-time backup, so backup safety cannot mask auto-repair.

## Startup service repair

Before opening the API or starting sync, the daemon compares its own installed
daemon definition with the current generator. It requires the saved config to
match this process and an active installed daemon service; timer installations,
other configs and foreground-only operation are left alone. The generator uses
saved installation inputs, not a transient shell environment.

On systemd it writes definitions atomically, runs `systemctl --user daemon-reload`,
then releases the daemon lock and exits **75**. The reloaded `Restart=on-failure`
policy starts the new command line. `daemon-reload` reloads units without
restarting the process ([systemctl manual](https://www.freedesktop.org/software/systemd/man/latest/systemctl.html#daemon-reload)).
A detached child alone would not escape systemd's default control-group cleanup,
so Linux never uses that approach.

A rewritten launchd plist needs bootout/bootstrap
([launchctl manual](https://keith.github.io/xcode-man-pages/launchctl.1.html)).
The macOS daemon starts a separate `kitbag --config <config> service refresh`
helper with a new session/process group and no inherited API/lock descriptors.
The helper does the full rewrite and supervisor replacement; the parent keeps
serving until launchd stops it. This avoids having bootout kill the process that
still needs to bootstrap the job. launchd cleans up processes in the original
job's process group ([AbandonProcessGroup](https://keith.github.io/xcode-man-pages/launchd.plist.5.html#AbandonProcessGroup)).
Helper output and failures go to `<log>/service.refresh.log` (0600); like the
supervisor output streams this file is not rotated. The helper is reaped if the
parent survives. Real launchd/systemd behavior remains
unverified; platform stubs and subprocess tests verify the requested operations.

A private `<state>/service-refresh.json` records attempt times **before** the
supervisor work. Startup attempts are limited to once per **10 minutes per
binary version**, including failed attempts and backwards clock changes. A new
version has its own budget. A pending record keeps failed rewrite/reload work
visible as stale. Failures/deferred attempts leave the daemon usable; explicit
`service refresh` bypasses the automatic cooldown. No recurring automatic repair
is scheduled: a deferred startup repair waits for another startup or an explicit
refresh. Neither marker contains credentials.

API v1 status and events carry optional `daemon_version`, the serving process's
version, independently of the client binary and API version. Clients fall back
to existing `version.get` for older v1 daemons. Missing version metadata or an
unavailable version handshake identifies an active supervised daemon as older.
Both service install and refresh restart that process even if the definition
has not changed. `kitbag status`, doctor and the
TUI show `daemon X, binary Y` and warn on a mismatch; they never restart a service
implicitly. A daemon predating executable watching still needs the first explicit
refresh/confirmed TUI action after upgrading.
