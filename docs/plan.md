# Logical synchronization plans

`internal/core/plan.Build(platform.FS, Request)` is the common policy boundary
for future plan, status and UI commands. It reads only through the supplied FS,
never writes, and has no clock, randomness, environment reads, processes or real
adapter dependency. The caller validates the manifest, checks the tool version,
resolves its host, loads validated state, and supplies the complete adapter set.
Call under the state lock when preparing an apply; FS reads are not a snapshot
and the apply engine must recheck preconditions before mutation.

`internal/target` is the shared adapter-facing contract. An adapter receives the
host-resolved `pkg/manifest.Manifest`, repository root, and read-only filesystem,
and returns absolute target declarations with primitive/agent-instance owners:

- `materialized`: one repository file or directory, independent of link/copy mode.
- `generated`: exact output bytes, such as rendered instructions.
- `merged`: a source fragment or in-memory fragment plus a pure native-format
  `Merger`. It receives current bytes (nil if absent) and fragment bytes, preserves
  unowned keys, and rejects unsafe input. No real merger is implemented here.
- `Warning`: an unsupported target produces a skip and protects previous state.

Core gathers declarations, hashes repository trees, unions owners for identical
shared destinations, and rejects different sources/contents, case-fold aliases,
and ancestor/descendant desired destinations. It uses Unicode simple case folding
(`strings.EqualFold`), even on the case-sensitive fake/Linux FS. Shared merged
outputs require one combined declaration: independently composed mergers at the
same destination are rejected because their policy/order cannot be inferred.
Adapters remain responsible for enabled primitive selection, native rendering,
source enumeration and reporting unsupported formats. They must omit removed
entries. A still-declared, missing source is also treated as absent desired state;
it stays absent on subsequent plans. Other unsupported source entries produce a
protective skip. Adapters can declare an explicit Warning for required sources.
Arbitrary files and Git includes will need declarations from their eventual implementation too; this
change implements no real adapter, CLI wiring or execution.

## Hashes and ownership

Hashes use the state's `sha256:` plus lowercase digest envelope:

- Files/generated/merged output: SHA-256 of `kitbag-file-v1` followed by a NUL byte
  and the exact file bytes. Empty files and line endings are significant.
- Directories: SHA-256 of `kitbag-tree-v1` followed by NUL and compact JSON for an
  array of `{name,hash,directory}` child records, sorted by name (Go string byte
  order). Child hashes recurse by these same rules; empty directories count.
- Separate domains prevent a file containing a directory's encoded preimage
  from masquerading as that directory. Permissions and timestamps do not count.
- Source links, links inside directory trees, special files and case-folded
  sibling collisions are unsupported in both modes. Intermediate source aliases
  are refused. A source's disappearance is distinct from an unreadable source:
  permission/I/O errors fail the build with no partial plan.

This is a **local materialization hash v1**, not a Git tree hash, skills CLI
`computedHash`, APM hash or remote integrity/provenance claim. No lock interchange
or resolution behavior is introduced ([ADR 9](adr/0009-skill-lock-interchange.md)).

State ownership is necessary even when unmanaged bytes equal desired bytes.
For a recorded link, its resolved lexical target must equal the recorded
`SourcePath`; merely pointing somewhere inside the repository is insufficient.
An unchanged link identity establishes ownership even when dangling. Foreign or
retargeted links are never followed for destination hashing or backed up.
Destination parent links/non-directories and existing case-only aliases produce
skips; the planner does not traverse them to write through another name.

## Three-way decision table

For files and trees, S is the desired source/output hash, W the last written
hash, and C the current destination hash. P is the previously applied source
hash (`SourceHash`) when a source exists; otherwise P = W. Kind compatibility is
`materialized` with either recorded `link` or `copy`, `generated` with `generated`,
and `merged` with `merged`. A source-path or logical-kind change also needs an
update even if bytes are equal. Owner-only changes use `record`.

The rows below are evaluated after path/type safety checks, in this order:

| Ownership / condition | Classification | Operations |
| --- | --- | --- |
| Unsafe destination/parent, foreign link, unsupported entry | Conflict if managed; unmanaged obstacle otherwise | Skip with warning; retain state/content |
| Desired target, destination missing | Update needed | Create desired output |
| No recorded owner, existing regular file, backup policy unset | Unmanaged obstacle | Skip, even if C = S |
| No recorded owner, existing regular file, `Backup: Once`, unused backup name | Unmanaged obstacle | Backup then create desired output |
| No recorded owner, directory, or backup name occupied/reserved | Unmanaged obstacle | Skip; explicit adoption is required |
| Managed compatible target, C = W, S = P | In sync | None; record if ownership/hash metadata differs |
| Managed target, C = W, S != P (or path/kind changed) | Update needed | Replace desired output |
| Managed target, C != W, C = S | Update needed (converged) | Refresh desired output and metadata |
| Managed target, C != W, C != S, S = P | Local drift | Skip with warning |
| Managed target, C != W, C != S, S != P | Conflict | Skip with warning |
| Unsupported declaration or unsafe/rejected merge | Conflict | Skip with warning; protect prior ownership |

For an ordinary copied/materialized target P = W after apply. Source fragments
for merges can differ from their output: stored `SourceHash` describes the input
fragment, and `WrittenHash` describes the rendered output. Generated/in-memory
fragments have no source fields in state; their prospective output supplies S.

**Settings merge:** the adapter first computes prospective output from current
bytes and its fragment. If that output already equals C, classify in sync and
record new metadata when needed, including allowed local-key additions. Otherwise
use the table: a simultaneous source update and destination edit conservatively
conflict; local drift is not silently discarded. In-memory merged fragments use
the prospective output versus W to detect change. A source fragment uses its
hash versus P. Format-specific reconciliation/force belongs to later work; this
engine does not implement an unconditional overwrite of locally edited settings.

**Links:** current contents naturally reflect working-copy edits, so comparing a
live source-backed C directly to W would make link and copy plans disagree. For
a still-owned link, normalize effective C to W and compare the new source S to P.
Thus a source update produces the same `materialize` operation as an unchanged
copy needing that update. Unchanged sources produce empty plans in both modes.
A destination link retargeted elsewhere is a conflict; editing through a valid
link is a source change, per ADR 3. Physical link/copy policy is intentionally
absent from the logical plan. KIT-16 must resolve manifest/machine mode precedence
and use Link or copy execution for `materialize`, without a silent fallback.

## Removals, moves and backups

A state target absent from desired state is stale:

| Current stale entry | Classification | Operation |
| --- | --- | --- |
| Missing | Update needed | Remove ownership only (`remove` is safe when already absent) |
| Recorded link still points to its recorded source, including dangling | Update needed | Remove link and ownership, never the source |
| Regular file/tree C = W | Update needed | Remove unchanged managed output and ownership |
| Retargeted link, unsupported entry, C != W or unsafe parent | Conflict | Skip with warning; never remove drifted/unmanaged content |

Moving a skill yields remove-old and create-new in one plan. There is no second
sync to discover the old target. A source move retaining the destination needs
one replacement. Stale ancestor links/files can be removed before creating a new
child; an unsafe stale ancestor blocks that creation. Unchanged case-only moves
are likewise removed before creation. Parent directories left behind by managed
children are not themselves owned, so replacing such a directory needs explicit
adoption. File/directory shape changes with an existing directory are also
conservative conflicts; no implicit recursive adoption occurs.

`Once` backs up only an unmanaged regular file to `<destination>.kitbag-backup`.
That name must be unused and must not overlap desired/state targets or the repo.
The plan never overwrites a backup and never creates a second numbered backup.
After apply, state ownership prevents another backup even after history rotation.
After a backup rename interrupted before output installation, the now-missing
destination can be created without repeating the backup. If ownership is lost
and an obstacle returns while the backup remains, skip. Retention, sensitive-file
permissions and real crash recovery belong to KIT-16/state policy.

## Order and rendering

Operations sort by phase: all removals, all backups, all output operations
(`materialize`, `write`, `merge`), all metadata-only records, then skips. Within a
phase sort by destination path in Go string order. Nested removals sort deepest
first. This makes every remove precede any overlapping creation and every backup
precede its replacement. Observations sort by destination and include in-sync
targets for status/UI use. Adapter order, maps, owner order and state order do not
change the result. `record` changes state only; `skip` never authorizes mutation.

`Plan.Text()` renders actions and warnings, or `No changes.` with a final newline.
`Plan.JSON()` has its own `plan_schema: 1`, arrays (including when empty), stable
indentation and a final newline. Both exclude generated/merged output bytes and
merger implementation details. JSON is a review/status representation, not a
serialized executable plan: apply needs a fresh in-memory plan under the lock.
Golden fixture I/O lives outside core in `internal/plancheck`; every tested
planner filesystem is the in-memory fake. Update reviewed goldens explicitly:

```sh
mise exec -- go test ./internal/plancheck -run '^TestPlanRenderings$' -args -update
```

The fake-only test executor lives in `_test.go`, not production. A deterministic
100-seed test generates repository trees, prior states, moves, deletions, missing
destinations and unmanaged files in both modes, applies a plan, asserts the next
plan has zero operations, and checks output contents independently. Conflict and
warning plans intentionally remain nonempty until resolved: pretending to apply
a skip cannot make local drift disappear. KIT-16 should reuse/replace the executor
in its own property test with actual apply. Real host behavior, real adapters,
execution atomicity and crash recovery are not verified by these tests.
