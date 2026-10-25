# 11. Keep daemon scheduling independent of host mechanisms

- Status: Accepted
- Date: 2026-10-25

## Context

Plan §2.1, §3 and §14.3 require inexpensive remote polling, drift notifications,
a local API and update awareness without an installer or heavy background host.
The existing workflow and API already provide synchronization safety and bounded
event delivery. Reimplementing either inside a daemon would create competing
policies and make shutdown/recovery harder to reason about.

## Decision

`internal/core/daemon` owns a single serialized scheduling loop over injected
clock/timer, remote probe, sync runner, drift inspector, update checker and event
publisher contracts. Native notifications only request read-only inspection;
remote changes invoke the existing workflow without force. Periodic inspection
is always enabled to recover from overflow and watch limits. The state lock
continues to exclude API and CLI operations across processes.

`vcs.Backend.RemoteHead` advertises the checked-out branch's configured upstream
without fetching objects. Both existing backends implement the same contract.
GitHub-specific ETag optimization for repository polling is deferred.

`internal/platform/watch` owns fsnotify, directory traversal limits and its
bounded hint collector. Platform code registers OS signals. `internal/daemonhost`
supplies real timers, cached release metadata policy and a small atomic
JSON log rotator through platform filesystem contracts. `internal/releasehttp`
confines release HTTP and offers a separate metadata-only entry point.
`Dirs.LogDir` follows
XDG state. The command wires these pieces, holds the daemon lifetime lock, checks
crash-stale endpoints, and runs `api.Serve` with the existing handlers.

## Consequences

- Pure scheduler tests use virtual clocks; real-process tests prove local-remote
  application, locking, native drift watching and SIGTERM behavior.
- Syncs cannot overlap in the scheduler; state locking also protects other entry
  points. Shutdown waits for apply's journaled operations.
- Bounded native watch coverage trades immediate detection of every path for a
  periodic full inspection. Large repositories require separate performance work.
- Release checks emit an existing API hint. They have no download/install path.
- The short Linux resource measurement is reproducible; the ten-minute runner02
  acceptance run remains manual. See [daemon.md](../daemon.md).
