## kitbag

Synchronize coding-agent configuration

```
kitbag [flags]
```

### Options

```
      --config string        Machine configuration file (default: <config>/config.toml)
      --git-backend string   Git backend: auto, system or embedded
  -h, --help                 help for kitbag
      --host string          Host override (before KITBAG_HOST and config host)
      --mode string          Materialization mode: link or copy (before KITBAG_MODE and config)
      --quiet                Suppress successful sync change output
      --verbose              Show backend selection and working copy diagnostics
```

### SEE ALSO

* [kitbag doctor](kitbag_doctor.md)	 - Explain synchronization problems and suggest fixes
* [kitbag plan](kitbag_plan.md)	 - Show the local synchronization diff without applying it
* [kitbag status](kitbag_status.md)	 - Show upstream relation, last sync and local drift
* [kitbag sync](kitbag_sync.md)	 - Pull and apply configuration in one pass
* [kitbag version](kitbag_version.md)	 - Print the tool version

