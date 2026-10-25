# Machine-local state store

`internal/core/state` owns `state.json` in `platform.Env.Dirs().State`, beside
`state.lock`. It accepts only `platform.FS` and `platform.Env`; it does not
resolve sources or execute plan, apply or revert operations.

## Schema 1

The JSON snapshot contains:

- `schema_version`: currently `1`, independent of manifest and provenance versions.
- `targets`: each absolute destination path, materialization `kind` (`link`,
  `copy`, `generated`, `merged`), local `source_path` and `source_hash`, last
  `written_hash`, and `owners` (primitive plus agent instance). Shared
  destinations have multiple owners. Generated and merged outputs may omit both
  source fields when they have no single source.
- `history`: applied plans, oldest first, each with a unique `id`, `applied_at`,
  and destination changes. Each change records `path` and `previous`: null for
  a newly created entry, otherwise its prior kind/hash and exact link target or
  file backup path. `previous.managed` optionally preserves the old target's
  ownership and source metadata; nil means the previous entry was unmanaged.

Path fields must be absolute and clean in the host's syntax. `link_target` is
an exception: it preserves the symbolic link's exact text, including relative
links, so revert can reproduce the original link. NUL bytes are rejected.
Hashes currently require `sha256:` plus 64 lowercase hexadecimal digits. The
store validates the envelope; the [plan engine](plan.md#hashes-and-ownership)
computes hashes and defines the representation of directories and generated
output. It stores neither file contents nor credentials. Local source paths/hashes describe applied working-copy
inputs, never canonical remote identities or resolved pins. Those belong in the
separate provenance contract ([ADR 9](adr/0009-skill-lock-interchange.md)).

Save sorts targets and changes by path and owners by agent instance/primitive,
normalizes timestamps to UTC, uses stable indentation and a final newline, and
writes empty collections as arrays. Plan order is retained. Save does not mutate
its caller's collections. Duplicate paths, owners and plan IDs are rejected.

The journal retains the last 10 plans by default; `Options.HistoryLimit` accepts
another positive limit (zero selects the default). The caller appends completed
plan records before Save. The store neither deletes external backups as history
expires nor guarantees they still exist; the future revert/backup policy must
retain referenced bytes. A backup can contain sensitive configuration, so its
storage and permissions belong to that future policy. No contents enter JSON.

## Lock and transaction boundary

Construct a store with `state.New(fs, env, options)`, then call
`store.WithLock(func(session *state.Session) error { ... })`. Inside that callback,
Load reads state, the caller performs its operation, and Save commits the new
snapshot and history together. All writers must use this same boundary, including
manual and timer invocations. Sessions reject Load/Save after the callback ends.
The lock is released on return, error or panic. A callback must not retain its
session or launch background work that outlives it.

Contention exits immediately with `platform.ErrLocked` and the message
"another sync is running; retry after it exits". This policy avoids a second
manual or scheduled operation silently waiting with a stale plan. Waiting or
retrying belongs to the caller, which must load and plan again under a new lock.

Stale detection uses the lock operation itself: success proves there is no live
holder, even if `state.lock` already exists. On Unix, `flock` releases on process
death. There is no PID/mtime timeout and no lock-file deletion: replacing/deleting
the locked inode can permit two writers. An alive but hung holder is not stale
and must not be stolen. The fake models acquisition/release and leftover files;
it does not simulate a kernel/process crash. This assumes a local state directory
and cooperating writers; network filesystems or external removal/replacement of
the lock file are outside that guarantee.

New directories use 0700; state and new lock files use 0600. Existing directories
are not chmodded. Save uses `FS.WriteAtomic`, so partial staging writes and
pre-rename failures preserve the previous snapshot. A failure syncing the directory
after rename can report an error with the new state already visible: reload
before retrying. The optional `pending` step is the write-ahead recovery contract used by
[apply](apply.md). It is validated and serialized here; recovery and host
execution remain in apply. Completed steps save ownership/history immediately.

## Reading and migrations

Only a missing file produces empty state. Malformed JSON, unknown fields,
invalid metadata and unsupported versions fail closed, including on Save; no
fallback overwrites an unreadable or newer database.

`Options.Migration` optionally upgrades explicitly versioned N-1 JSON in memory.
This release defines no legacy schema; a version-0 hook in tests demonstrates the
extension point. Missing versions and versions outside N/N-1 are rejected. The
hook must return current-version, valid JSON; errors, invalid output and no
version progress fail. Load never writes. An explicit Save preserves original
legacy bytes in `state.v<old-version>.json` before atomic replacement. An identical
archive permits retry after interruption; a conflicting or unreadable archive
blocks replacement. Current-version saves need no archive.

## Evidence

All state unit tests use the in-memory fake, including contention/lifetime,
leftover-lock reuse, bounded history, migration, deterministic serialization and
one-shot interruptions at partial-write and pre-rename stages. The shared platform
contract suite still runs against fake and Unix.

`cmd/kitbag/TestStateProcesses` is the real-host integration test (Linux/macOS).
It re-executes the test binary, uses pipes to hold one writer across a second
process's acquisition attempt, verifies the clear contention error, then checks
the persisted snapshot. It covers normal exit and forced process death; the
unchanged lock file can then be reacquired and state stays readable. Only this
integration test constructs a real host for the state store; core tests never
import `os` or a concrete host implementation.
