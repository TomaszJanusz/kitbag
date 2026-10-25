# Adopting, unmanaging and reverting

`kitbag adopt` takes over a machine that `agent-config-sync` already manages,
without touching a file. `kitbag unmanage` and `kitbag revert` are the ways
back. Policy lives in `internal/core/adopt` and `internal/core/undo`; both run
through the planner and the [apply engine](apply.md) under the state lock and
reach the host only through `platform.FS`.

## Migration walkthrough

1. **Point KitBag at the existing clone.** Adoption leaves bytes alone, so
   KitBag must render exactly what the script wrote: use the clone the script
   runs from as `repo.path`, and a `kitbag.toml` that declares the same layout
   ([example](../schemas/examples/kitbag/valid/agent-config.toml)).
2. **Declare the legacy roots.** Adoption refuses to run without explicit
   `migration.legacy_roots`; the real paths are a maintainer decision, never
   guessed:

   ```toml
   [migration]
   legacy_roots = ["~/src/agent-config", "~/.agents/skills"]
   ```

3. **Stop the old timer** (systemd user timer or LaunchAgent; `kitbag doctor`
   lists units whose names contain `agent-config`). Both tools running would
   fight once KitBag rewrites a file with its own marker.
4. **Preview, then adopt:**

   ```sh
   kitbag adopt --dry-run      # or --dry-run --json (adopt_schema 1)
   kitbag adopt
   kitbag plan                 # "No changes." when everything was adopted
   ```

   The report lists every adopted destination, what the next sync would still
   change (`remaining`, the plan after adoption) and legacy links beside
   declared destinations that the manifest does not declare (left alone).
5. **Optionally move real skill directories aside** with `--move-real-dirs`,
   as `agent-config-sync --adopt` did; the next sync links the skills.
6. Remove the script's own `~/.local/bin/agent-config-sync` link once the
   timer is gone; it has no KitBag counterpart and is never touched.

## What adoption accepts

Adoption records ownership only for unowned declared destinations that a fresh
plan, with that ownership, leaves **in sync**:

| Destination | Adopted when | Recorded |
| --- | --- | --- |
| Link (link mode) | its lexical target is exactly the declared source | `link`, source hash |
| Copy (copy mode) | its tree hash equals the source's | `copy`, source hash |
| Generated file | bytes equal KitBag's output, or the script's output with its `Managed by agent-config-sync (<repo>)` marker | `generated`, actual hash |
| Merged settings | bytes equal the merge of themselves, or that merge with `# agent-config-managed` instead of `# kitbag-managed` | `merged`, fragment and actual hash |

The written hash is always the hash of the bytes present, so adoption never
rewrites, backs up or deletes anything (asserted by `adopt_legacy.txtar` with
file modification times). Candidates come from `plan.Candidates`; adoption then
builds a plan with them owned and drops every candidate that still has an
operation, repeating to a fixpoint. Everything else stays unowned and appears
under `remaining` with the operation sync would perform: links into a legacy
root that point elsewhere are relinked, hand-written files are backed up once,
real directories are conflicts.

**Marker policy.** The legacy markers are accepted, not migrated. A declared
generated target carries `Target.Legacy` (the script's rendering) and a
settings merger implements `target.LegacyRenderer`; the planner treats a
destination holding exactly that rendering as in sync (see
[plan.md](plan.md#legacy-equivalence)). When the desired output changes, the
next sync writes KitBag's marker through the normal, visible update path,
keeping the adopted bytes in a backup. Rendering differs when the repository
path differs from the one in the script's marker (it used `pwd -P`); such a
file is reported as remaining instead.

Adoption is journaled as one plan whose changes carry `adopted: true` and no
previous entry. Reverting it forgets the ownership again, still without
changing a file.

`--move-real-dirs` moves an unowned real directory at a declared skill
destination to `~/.agents/skills-before-kitbag/<path relative to HOME>`, once:
an existing directory there is never replaced (conflict, exit 3), and a
destination outside HOME is skipped. Agents do not scan that directory. The
move is a journaled apply step, so `unmanage` and `revert` can put it back.

## Unmanage

`kitbag unmanage [--dry-run [--json]]` handles every owned target:

| Current destination | Operation |
| --- | --- |
| missing | `release`: forget ownership |
| changed since KitBag wrote it, unsafe parent | `skip` (conflict, exit 3); ownership kept |
| unchanged, an intact original backup is journaled | `restore`: discard KitBag's output, rename the original back |
| unchanged settings whose original merges exactly into them | `restore` of the original |
| other unchanged settings | `unmerge`: remove KitBag's hook groups in place, keep every other key |
| other unchanged output | `discard`: delete it |

"Original" means the newest journaled backup of an entry KitBag did not own (a
one-time backup or an adoption move) whose bytes still match the journal.
Discarded output is never backed up: the repository reproduces it, and a
backup sibling would block the next one-time backup. A fresh `sync` therefore
converges after `unmanage` (`unmanage.txtar`). Limitations: once history has
rotated past the one-time backup, the original is no longer found and the
output is simply discarded; settings with absorbed local edits keep their
original backup, which then blocks the next sync's one-time backup until it is
removed; merged repository keys cannot be told from local ones and stay.
Unmanage reads the manifest only to find the settings merger; without one the
settings are skipped.

## Revert

`kitbag revert [--to n] [--force] [--dry-run [--json]]` and `kitbag revert
--list [--json]` (history_schema 1). Each command's steps form one journaled
plan; `--list` numbers them newest first. Reverting `n` plans returns every
destination they changed to its entry before the oldest of them:

- **previous targets** are the journaled `previous` entries: absence, the exact
  link text, or a file/tree in its backup (hash rechecked), plus the previous
  ownership (`previous.managed`, or unmanaged);
- the destination must still match state (KitBag's own output, or absence);
  otherwise it is a conflict, unless `--force` moves it aside to a backup;
- an entry already holding the target needs only its ownership;
- unchanged current output is discarded; a target whose previous output was
  discarded (by unmanage or an earlier revert) is skipped with a hint to sync.

Revert is journaled too, so reverting again undoes it (links and backups come
back; discarded output does not). The next sync reapplies the manifest unless
the repository changes: revert is a local brake, not a repository revert. If a
revert is interrupted, the next revert only recovers the step and stops; its
completed steps are plan 1, so `revert --to 2` finishes the original revert.

## Exit codes

The [shared table](apply.md#reports-and-exit-codes) applies: 0 success; 1
warnings and operational errors; 2 usage (no legacy roots, `--json` without
`--dry-run`, `--to` beyond the journal); 3 conflicts (changed destinations,
an occupied move-aside directory). Dry runs exit 0.

## Verification

- `cmd/kitbag/testdata/script/adopt_legacy.txtar`: a HOME recorded from the
  real `agent-config-sync` is adopted; `treesnap` (paths, modes, sizes,
  modification times, hashes, link texts) and the Git configuration are equal
  before and after, `plan` reports no changes, `sync` is silent and changes
  nothing; a later source change rewrites the marker visibly.
- `unmanage.txtar`: originals restored, user files untouched, then a fresh
  sync converges and repeats silently; changed output is a conflict.
- `revert.txtar`: a bad change is reverted (bytes, links, absence), changed
  destinations need `--force`, which keeps them in a backup.
- Unit tests on the fake platform cover candidates, the fixpoint, moves,
  every unmanage/revert row and convergence after every injected filesystem
  interruption.

Regenerate the recorded HOME (read-only use of the script, throwaway HOME,
isolated Git configuration, no network):

```sh
python3 scripts/legacy-home.py /path/to/agent-config/bin/agent-config-sync
```
