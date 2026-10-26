# Applying synchronization plans

`internal/core/apply` executes through `platform.FS`; it has no OS, process,
environment or console access and never reads the clock itself. `Options.Now`
is a required injected `func() time.Time`. `Run(fs, store, request, options)` is
the internal entry for KIT-23: it locks state, recovers an interrupted step,
loads state, builds a fresh plan and calls `Execute`. Manifest validation,
`CheckVersion` and `ResolveHost` precede it. The public `sync` command uses the
locked-session entry points below so the same lock also covers pull, manifest
loading, Git includes and result recording.

Callers that already hold the state lock can use `Recover(fs, session)`, load,
`plan.Build`, then `Execute(fs, session, plan, options)` in that order. A plan
with pending recovery, missing preconditions or different force authorization
is rejected. Neither plans nor sessions may be reused after an apply error.

## Modes and policy

`ResolveMode(manifest, flag, environment, configuration, platformDefault)` selects
flag → environment → configuration → platform default (empty defaults to Unix
`link`), then lets `copy` on either the manifest or machine side win. It performs
no environment reads. Reuse `Manifest.ResolveHost` for host/host-target overrides.
Adapters use `target.ManifestMode` for target → instance → target primitive →
instance primitive policy, or the file entry's explicit mode, and put the result
in `Target.Mode`. Empty declarations inherit the resolved manifest default.
`Run` supplies the selected machine mode to planning so an existing link/copy
can change representation. Low-level `Build` callers supply `Request.Mode` for
representation changes; leaving it empty retains mode-independent logical plans.
A link failure is an error, never an implicit copy fallback.

Only planner-authorized operations execute. Unmanaged directories and foreign
links are conflicts, including with force; no recursive adoption is implemented.
An unmanaged regular file needs `Backup: target.Once`. Managed materialized copy
drift warns and remains unchanged without force; simultaneous source/local edits
conflict. `Options.Force` is also passed to the planner by `Run`; a force plan can
back up and replace drifted owned files/trees only when its declaration permits
`Once` and that backup namespace remains unused. An existing one-time backup
still blocks another force backup. Generated/merged settings force semantics
remain outside this ticket.

Missing `Target.InstanceDir` produces a warning and protects prior ownership;
other targets continue. This optional field distinguishes a listed agent instance
from destination parents that apply may create. Adapters must populate it for
listed instances. It is checked again before output mutation.

Legacy migration is an explicit planner input: `Request.LegacyRoots` contains the
known old agent-config clone and `~/.agents/skills` roots resolved by the caller.
With migration roots configured, links lexically inside those whole-component
roots (including relative links), and any dangling link, are replaceable, matching
the legacy `replaceable` rule. Targets are never modified or followed for ownership;
Stat is used only to identify dangling links. Links into the active clone can be
included too. Files holding exactly agent-config-sync's rendering of the desired
output, for the active clone or a clone at a legacy root, are replaceable too
([plan.md](plan.md#legacy-equivalence)). Both are replaced in place, without a
backup. Without migration roots the strict KIT-15 foreign-link rule remains.
Core never guesses clone locations or reads HOME. The CLI supplies only explicitly
configured `migration.legacy_roots` (default: none).

## Preconditions and backups

The planner binds a non-serialized precondition to each operation: absence, or
entry kind plus content hash/exact link text. It models earlier removals/backups
when binding later operations. Apply rechecks this identity, source hashes,
parent safety and case aliases before work, then checks again after staging and
immediately before destination mutation. A changed target is a conflict; a failed
backup blocks that path's subsequent replacement. The one-time backup namespace
is rechecked at execution too. `skip` never grants mutation permission.

Backups use `<destination>.before-kitbag.<UTC time>-<sequence>`. Time has format
`20060102T150405.000000000Z` (Go layout), for example
`AGENTS.md.before-kitbag.20260102T030405.000000000Z-0`. The sequence avoids collisions
with earlier completed steps at the same injected time; it never permits a second
unmanaged one-time backup. Any sibling starting with `.before-kitbag.` after the
destination name, or the older exact `.kitbag-backup` name, blocks another one-time
backup, even after state/history loss. Matching is case-insensitive. Backups are
renamed, never overwritten. Recovery storage may not overlap managed targets. Apply rejects destinations
overlapping the state directory or its ancestors, protecting state and its lock.

Owned file/tree replacements and removals also retain the prior entry in a
uniquely named backup for the state journal. Unmanage and revert plans instead
*discard* unchanged owned output: it moves to a reserved `.kitbag-stage.<step ID>`
sibling that is deleted once the step completes, so no backup sibling blocks a
later one-time backup. Output replacing a link discards the link the same way,
owned or legacy: the journal keeps its exact text, which is all revert needs, and
a backup sibling of a skill link would be one more skill that agents load. Output
replacing agent-config-sync's rendering (`Operation.Replaces`) discards it too,
journaled as previously owned output that the repository reproduces; revert of
that step therefore reports it as discarded instead of restoring the old marker. A forced revert moves a changed destination to a backup. No backup is automatically pruned when history
rotates. Existing permissions are retained by rename. New directories and files
use 0700/0600; copies also retain source executable bits. Generated and merged
content is always a regular file: it keeps the permission bits of the regular
file it replaces (also when that file was just backed up), and is private (0600)
when it is new or replaces a link. Directory copies are staged as complete trees;
source links/special files and changing source content are refused.

## Interruption and durability

Before each host step, Save atomically records one `Database.Pending` intent
containing the expected entry, intended target, owned staging sibling, backup
or discard sibling, plan group and prior metadata. It contains hashes/paths,
never file bytes. Step actions are `output` (optionally `release`: the written
settings become unowned, used by unmerge), `remove`, `backup` (an adoption move
may name `~/.agents/skills-before-kitbag/...` instead of a sibling) and
`restore` (rename a journaled backup `from` back, or recreate a `restored` link,
then record or drop the journaled ownership). Each Execute groups its journal
entries under the first step's ID. Outputs
are built under `<destination>.kitbag-stage.<step ID>`, checked, the old entry is
renamed aside, then the staging entry is renamed into place. Successful steps
immediately Save ownership/removal plus a history entry and clear Pending before
the next step. Metadata-only records Save immediately. History entries
represent completed steps grouped into plans; the state history limit counts
plans.

Recovery recognizes complete intended output (or the restored entry) and
records its ownership, or restores the original entry from its backup or discard
sibling and discards incomplete staging before replanning.
An interrupted backup/remove is finalized when its destination is absent. A
changed external destination blocks recovery rather than discarding local edits.
Recovery never needs old source bytes or a serialized executable plan. Backups
survive recovery; only reserved staging created by this transaction is removed.
No work is added to state.Load/Save beyond validation and serialization: state
still has no host execution policy. Optional pending metadata extends schema 1;
older readers fail closed on the unknown field while recovery is outstanding.

This is convergence across process interruption and reported FS errors, including
post-rename errors, under the platform contract. Cooperating KitBag writers use
the state lock. Arbitrary other programs can still race a check and a rename:
platform.FS offers no compare-and-swap or directory-handle API. Other programs
must not create/change reserved backup/staging names. Replacement
briefly has an absent destination between its two renames. Physical power-loss
recovery on every filesystem is not claimed.

## Reports and exit codes

`Result` contains successful `Changed` operations, skipped `Warnings`, and
`Conflicts`. `record` and `release` operations change ownership only; a
`record` carrying a restoration owner records exactly that journaled target. `Result.Code()` gives conflicts priority. `ErrorCode(err)` handles
operational, usage, tool-version and unsupported-OS errors; callers combine codes
with `max`. Empty successful results render nothing to either stream, exit 0,
and do not rewrite state. An error stops execution and requires recovery/replan.

| Code | Meaning |
| --- | --- |
| 0 | OK, including unchanged repeat run |
| 1 | Warnings (missing instance, local copy drift), or operational failure |
| 2 | Usage/invalid apply options |
| 3 | Conflict or changed precondition |
| 4 | Manifest requires a newer tool (`manifest.VersionError`) |
| 5 | Unsupported OS; existing Windows refusal before 1.x |

The single code definition is `apply.Code`; cmd reuses it for its existing
failure, usage and Windows exits. [platform.md](platform.md) retains code 5.

## Verification and CLI boundary

The planner's 100-seed property runs production Execute for both modes, checks
apply → replan is empty and independently compares desired contents. Fake tests
interrupt every state write (both partial-write and pre-rename faults), every
filesystem mutation boundary (including errors after rename), and a partial
nested copy. They recover/replan/apply and compare targets, contents, preserved
original backup, absence of stale destinations/staging, and a silent repeat with
an uninterrupted run. Per-step history checks catch delayed persistence.

The test binary alone recognizes `kitbag __e2e_apply`, implemented in
`cmd/kitbag/apply_driver_test.go` with a tiny fixture adapter. Testscript launches
that real process under the existing HOME guard, uses a minimal real manifest
and real Unix FS/state, and tests first run, silent repeat, missing instance,
real-directory conflict, one-time backup, drift and forced replacement. Its
`execstatus` helper asserts exact binary exit status as well as stdout/stderr.
It is absent from installed binaries. Fixture-only scripts retain that driver;
real-adapter scripts now exercise public `sync --no-pull`. See [commands.md](commands.md) for CLI orchestration.
