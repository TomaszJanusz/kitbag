## kitbag sync

Pull and apply configuration in one pass

### Synopsis

Fetch and fast-forward a clean working copy, then read its manifest and apply one plan. Local changes skip the pull. An unchanged successful run is silent.

```
kitbag sync [flags]
```

### Options

```
      --force     Replace owned materialized drift when one-time backup policy permits
  -h, --help      help for sync
      --no-pull   Use the existing working tree without fetching
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

