## kitbag plan

Show the local synchronization diff without applying it

### Synopsis

Build a dry-run plan from the existing working tree. No fetch or recovery is performed. The logical target diff is coloured only on a terminal with NO_COLOR unset. JSON excludes file contents.

```
kitbag plan [flags]
```

### Options

```
  -h, --help   help for plan
      --json   Print stable plan_schema=1 JSON
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

