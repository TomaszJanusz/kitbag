## kitbag service status

Show supervisor health and the last synchronization

```
kitbag service status [flags]
```

### Options

```
  -h, --help   help for status
      --json   Print stable service_schema=1 JSON
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

* [kitbag service](kitbag_service.md)	 - Manage background synchronization for this user

