# Guide: migrating from agent-config-sync

`kitbag adopt` takes over a machine that the `agent-config-sync` script already
manages, without rewriting a file. This guide is the short path; the contract is in
[adopting, unmanaging and reverting](adopt.md).

## Before you start

You need a working `kitbag` ([install](../README.md#install)) and a repository
that kitbag can read: your existing agent-config repository plus a `kitbag.toml`
declaring the same layout as the script produces
([example](../schemas/examples/kitbag/valid/agent-config.toml)). Commit and push that
manifest first.

1. **Stop the old timer.** Both tools running would fight over a file once
   kitbag writes its own marker. `kitbag doctor` lists units whose names contain
   `agent-config` that are still loaded or enabled. Disable the systemd user timer
   or the LaunchAgent you created for the script, for example
   `systemctl --user disable --now <name>.timer`, or `launchctl bootout` and
   `launchctl disable gui/$(id -u)/<label>` (booted out alone, it loads again at login).
2. **Connect the repository.** `kitbag init <git-url>` clones it into kitbag's
   own working copy. To keep using the clone the script ran from, set it as
   `repo.path` instead (`kitbag config set repo.path ~/src/agent-config` after
   creating a configuration with `init` or `config set repo.url`); adoption then
   leaves every byte alone. See
   [choosing the working copy](adopt.md#migration-walkthrough) for the difference.
3. **Declare the legacy roots.** Adoption refuses to run without them; kitbag never
   guesses. List the old clone and the old skills directory:

   ```sh
   kitbag config set migration.legacy_roots '["~/src/agent-config", "~/.agents/skills"]'
   ```

4. **Set the host the script used** when it is not the machine's short
   hostname (for example the script's `hosts/mac.md` on a Mac named `MBP-TJ`):
   `kitbag config set host mac`. Otherwise the host addition is missing; every
   command warns and names the repository's host files.

## Adopt

```sh
kitbag adopt --dry-run     # report only; add --json for adopt_schema 1
kitbag adopt               # record ownership; no file is rewritten
kitbag plan                # "No changes." when everything was adopted
```

The report lists every adopted destination, what the next sync would still change
(`remaining`) and legacy links that the manifest does not declare (left alone).
`--move-real-dirs` additionally moves real skill directories aside once, as
`agent-config-sync --adopt` did; see [adopt.md](adopt.md#what-adoption-accepts).

## Parity check

1. `kitbag plan` should print `No changes.`. If it lists operations, they are
   what `sync` would do: links into a legacy root that point elsewhere are
   re-linked, the script's renderings for an old clone are rewritten with kitbag's
   marker, hand-written files are backed up once, real directories are conflicts.
2. `kitbag sync` should then change nothing for adopted destinations and a second
   run should print nothing. When the working copy differs from the old clone, the
   first sync re-points links and rewrites `CLAUDE.md`, `AGENTS.md` and
   `settings.json` once, keeping any hand-edited file in a backup (see
   [separate working copy](adopt.md#what-adoption-accepts)). A change in the
   repository later rewrites those files visibly.
3. `kitbag doctor` should report no FAIL; its `legacy` check shows what is left.

## Switch the timers

Replace the script's timer with kitbag's own service:

```sh
kitbag service install      # daemon mode; or --mode timer --interval 15m
kitbag service status
```

On Linux a headless machine needs `loginctl enable-linger <user>`, which kitbag
prints but never runs; see [background synchronization](service.md).

## Remove the old script

Once the old timer is gone and `kitbag sync` is silent, delete the script's clone
only if you no longer use it as `repo.path`, and remove the script's own
`~/.local/bin/agent-config-sync` link yourself. kitbag has no counterpart for that
link and never touches it.

## What adoption does and does not guarantee

- It records ownership only for destinations that are already in sync; nothing is
  rewritten, backed up or deleted. Everything else stays unowned and is reported
  under `remaining`.
- The script's `Managed by agent-config-sync` and `# agent-config-managed`
  markers are accepted as equivalent; kitbag writes its own marker the next time
  the content has to change.
- The script's change to your global Git configuration (`include.path`) stays;
  kitbag adds its own include only if missing and never removes one.
- `kitbag revert` and `kitbag unmanage` are the ways back. Reverting an adoption
  only forgets ownership; it changes no file. Limits: a link or rendering that a
  later sync replaced in place has no backup (the repository reproduces it), and
  settings with absorbed local edits keep their original backup, which blocks the
  next one-time backup until you remove it. See [unmanage](adopt.md#unmanage) and
  [revert](adopt.md#revert).

## See also

[Generated reference for `adopt`](cli/kitbag_adopt.md),
[`doctor`](cli/kitbag_doctor.md), [`plan`](cli/kitbag_plan.md),
[`revert`](cli/kitbag_revert.md), [plan semantics](plan.md#legacy-equivalence),
[ADR 2](adr/0002-git-source-of-truth.md).
