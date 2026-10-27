## kitbag manifest upgrade

Convert kitbag.toml to manifest schema 2 and publish it

### Synopsis

Convert the shared kitbag.toml from schema 1 to schema 2 without losing
anything: schema becomes 2, min_tool_version becomes at least 0.1.4 (the first
release that reads schema 2), and each [hosts.<h>.targets.<t>.instances] table
that replaces a target's instances becomes the target-level hosts list plus
host patches. Comments, order and layout stay; the result is checked to give
every host the file names, and one it does not, the same active instances.
Layouts that cannot be converted exactly are refused with a hint to edit by
hand. kitbag config and config.toml are not affected.

Every machine that syncs the repository must run kitbag 0.1.4 or later
BEFORE the upgrade is published: older releases stop syncing (exit 4, or exit 2
for 0.1.0) and apply nothing until they are updated.

Publication works like kitbag agent add: the working copy must be clean, on its
configured branch and without unpublished commits; KitBag fast-forwards it,
commits the change as "chore(kitbag): upgrade manifest to schema 2" and, when this
host's publish mode is direct, pushes without force, then synchronizes. With
publish mode pr nothing is written. A preview is shown first; without a
terminal pass --yes, or --dry-run to only preview.

```
kitbag manifest upgrade [flags]
```

### Options

```
      --dry-run   Show the conversions, diff and commit message without writing
  -h, --help      help for upgrade
      --json      Print stable manifest_upgrade_schema=1 JSON (needs --yes or --dry-run)
      --yes       Apply without asking for confirmation
```

### Options inherited from parent commands

```
      --config string        Machine configuration file (default: <config>/config.toml)
      --git-backend string   Git backend: auto, system or embedded
      --host string          Host override (before KITBAG_HOST and config host)
      --mode string          Materialization mode: link or copy (before KITBAG_MODE and config)
      --quiet                Suppress successful sync change output
      --verbose              Show backend selection and working copy diagnostics
```

### SEE ALSO

* [kitbag manifest](kitbag_manifest.md)	 - Operate on the shared kitbag.toml

