## kitbag adopt

Take over destinations agent-config-sync already manages

### Synopsis

Record ownership of every declared destination that already holds what sync would write: links to the exact source, identical files and agent-config-sync's marker-only variants. No file is rewritten, backed up or deleted, so the next sync has nothing to do for them; anything else is listed as remaining. Requires migration.legacy_roots in config.toml. --move-real-dirs moves real skill directories aside once, as agent-config-sync --adopt did.

```
kitbag adopt [flags]
```

### Options

```
      --dry-run          Report what would be adopted without saving state
  -h, --help             help for adopt
      --json             Print stable adopt_schema=1 JSON
      --move-real-dirs   Move real skill directories at declared destinations aside once
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

