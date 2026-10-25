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

* [kitbag adopt](kitbag_adopt.md)	 - Take over destinations agent-config-sync already manages
* [kitbag config](kitbag_config.md)	 - Inspect and change machine settings
* [kitbag doctor](kitbag_doctor.md)	 - Explain synchronization problems and suggest fixes
* [kitbag init](kitbag_init.md)	 - Set up a configuration clone or scaffold a new repository
* [kitbag plan](kitbag_plan.md)	 - Show the local synchronization diff without applying it
* [kitbag revert](kitbag_revert.md)	 - Restore the targets as they were before recent plans
* [kitbag self-update](kitbag_self-update.md)	 - Verify and install a newer release or show the package manager command
* [kitbag status](kitbag_status.md)	 - Show upstream relation, last sync and local drift
* [kitbag sync](kitbag_sync.md)	 - Pull and apply configuration in one pass
* [kitbag unmanage](kitbag_unmanage.md)	 - Remove everything KitBag created and restore what it replaced
* [kitbag version](kitbag_version.md)	 - Print the tool version

