## kitbag status

Show upstream relation, last sync and local drift

### Synopsis

Report ahead/behind against the cached upstream, the last sync attempt for this clone, drift and warnings. This command never fetches; use sync to refresh remote knowledge.

```
kitbag status [flags]
```

### Options

```
  -h, --help   help for status
      --json   Print stable status_schema=1 JSON
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

