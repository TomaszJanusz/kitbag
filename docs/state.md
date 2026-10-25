# Machine-local state store

`internal/core/state` owns `state.json` in `platform.Env.Dirs().State`, beside
`state.lock`. It accepts only `platform.FS` and `platform.Env`; it does not
resolve sources or execute plan, apply or revert operations.

## Schema 2

The JSON snapshot contains:

- `schema_version`: currently `2`, independent of manifest and provenance versions.
- `targets`: each absolute destination path, materialization `kind` (`link`,
  `copy`, `generated`, `merged`), local `source_path` and `source_hash`, last
  `written_hash`, and `owners` (primitive plus agent instance). Shared
  destinations have multiple owners. Generated and merged outputs may omit both
  source fields when they have no single source.
- `history`: journal entries, oldest first, each with a unique `id`, `applied_at`,
  an optional `plan` and destination changes. Apply writes one entry per
  completed step; all entries of one command (sync, unmanage, revert, an
  adoption move) share `plan`, the first step's ID, and the history limit counts
  these plans. An entry without `plan` (schema 1) is its own plan. Each change
  records `path` and `previous`: null for a newly created entry, otherwise its
  prior kind/hash and exact link target or file backup path.
  `previous.managed` optionally preserves the old target's ownership and source
  metadata; nil means the previous entry was unmanaged. A file without
  `backup_path` requires `managed`: unchanged KitBag output that unmanage or
  revert discarded because the repository reproduces it. A change with
  `adopted: true` and a null `previous` records ownership taken over by
  [adoption](adopt.md) without changing the entry.

Schema 2 is a superset of schema 1, so the built-in `V1Migration` (the default
`Options.Migration`) only rewrites `schema_version`; the first Save archives the
original as `state.v1.json`. Older releases refuse schema 2 state.

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

The journal retains the entries of the last 10 plans by default;
`Options.HistoryLimit` accepts another positive limit (zero selects the
default). The caller appends completed entries before Save. The store neither
deletes external backups as history expires nor guarantees they still exist;
revert and unmanage recheck a backup's hash before renaming it back and skip a
missing or changed one. Backups keep the destination's permissions (private for
new files). No contents enter JSON.

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

`Options.Migration` upgrades explicitly versioned N-1 JSON in memory; it
defaults to `V1Migration`, and tests may inject another hook. Missing versions
and versions outside N/N-1 are rejected. The
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

## Last sync attempt (KIT-23)

`Session.LoadSync` / `SaveSync` access a separate `last-sync.json` beside
`state.json`, under the same state lock. It has independent `schema_version: 1`,
UTC `at` (attempt completion), absolute `repository`, integer `result` (the
[apply exit code](apply.md#reports-and-exit-codes)), and nonnegative `changed`
(completed apply operations plus Git includes added). It contains no file bytes,
credentials or diagnostic text. Missing means no recorded attempt. Status shows
`last_sync: null` when absent or when the record belongs to another clone.

Every sync that acquires the lock and reads valid record metadata records its
result, including an unchanged success, skipped dirty pull, divergence, fetch
failure, version refusal or partial apply failure. Configuration/backend failures
before entering the workflow and lock contention cannot be recorded. A dirty
pull skip alone is informational (exit 0); divergence exits 3 without applying.
This is one machine-wide latest attempt, not a per-repository history.

The record uses 0600 and `WriteAtomic`; unknown fields, malformed metadata and
unsupported versions fail closed and block overwrite. It does not change the
ownership schema, rotate history, or rewrite `state.json` on an unchanged sync.
A record write failure fails the command; completed file operations remain in
their own journal. As with state saves, a post-rename durability error can mean
the new record is already visible. Dry-run/status commands never save the record
or recover a pending step; they may create the private state directory/lock.
