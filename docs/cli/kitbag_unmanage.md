## kitbag unmanage

Remove everything KitBag created and restore what it replaced

### Synopsis

Delete unchanged links, generated files and copies that KitBag owns, remove its hook groups from merged settings, and rename one-time backups back into place. Changed or unowned entries are left alone and reported as conflicts. The steps are journaled; a fresh sync manages the machine again.

```
kitbag unmanage [flags]
```

### Options

```
      --dry-run   Print the unmanage plan without changing anything
  -h, --help      help for unmanage
      --json      With --dry-run, print stable plan_schema=1 JSON
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

