## kitbag revert

Restore the targets as they were before recent plans

### Synopsis

Undo the latest journaled plan (or the latest --to <n> plans) by restoring each changed destination's previous link, backup or absence and its ownership. Destinations changed since are conflicts unless --force moves them aside to a backup. --list shows the journal, newest first. The next sync reapplies the manifest unless the repository changes.

```
kitbag revert [flags]
```

### Options

```
      --dry-run   Print the revert plan without changing anything
      --force     Move destinations changed since aside to a backup instead of skipping them
  -h, --help      help for revert
      --json      With --dry-run or --list, print stable JSON
      --list      List journaled plans, newest first
      --to int    Revert the latest n journaled plans (see --list) (default 1)
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

* [kitbag](kitbag.md)	 - Synchronize coding-agent configuration

