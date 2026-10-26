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
2. **Connect the repository**, keeping the clone the script ran from:

   ```sh
   kitbag init <git-url> --repo-path ~/src/agent-config --no-service
   ```

   Skip the background service for now (`--no-service`, or answer no in the
   wizard): it syncs as soon as it starts, and adoption must come first.

   The clone's `origin` must be `<git-url>` exactly. Init then uses it as
   `repo.path`, makes no second clone, and adoption leaves every byte alone.
   Without `--repo-path`, init clones into kitbag's own working copy instead
   (see [choosing the working copy](adopt.md#migration-walkthrough)); setting
   `repo.path` afterwards leaves that clone unused, which `kitbag config set`
   points out but never deletes. Without a terminal (SSH command, script),
   `init` needs `--yes`, which accepts the defaults and writes the configuration
   after printing the dry plan.
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
kitbag adopt --no-service  # record ownership; no file is rewritten
kitbag plan                # "No changes." if repo.path is the old clone
```

Without `--no-service`, adopt offers the background service afterwards (yes by
default on a terminal); its first sync applies what remains, so install it after
the parity check below.

The report lists every adopted destination, what the next sync would still change
(`remaining`) and legacy links that the manifest does not declare (left alone).
`--move-real-dirs` additionally moves real skill directories aside once, as
`agent-config-sync --adopt` did; see [adopt.md](adopt.md#what-adoption-accepts).

## Parity check

1. With the old clone as `repo.path`, `kitbag plan` should print `No changes.`.
   With a separate working copy it lists the one-time replacements described in
   the next step. Any operations listed are what `sync` would do: links into a legacy root that point elsewhere are
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

Replace the script's timer with kitbag's own service (`kitbag adopt --service`
installs the same daemon):

```sh
kitbag service install      # daemon mode; or --mode timer --interval 15m
kitbag service status
```

Until it is installed, `kitbag doctor` warns that the service is missing.

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
  kitbag adds its own include only if no existing one names the same file
  (compared after expanding `~` and resolving symbolic links) and never removes
  one. With a separate working copy the script's include names the old clone's
  file, so both stay until you remove the script's yourself.
- Rewritten `CLAUDE.md`, `AGENTS.md` and `settings.json` keep their file mode;
  new files are created private (0600).
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
