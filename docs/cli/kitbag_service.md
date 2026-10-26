## kitbag service

Manage background synchronization for this user

### Synopsis

Install, uninstall or inspect a systemd user service or macOS LaunchAgent. Configuration, working copies and sync state are kept on uninstall.

### Options

```
  -h, --help   help for service
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
* [kitbag service install](kitbag_service_install.md)	 - Enable background sync with a daemon or timer
* [kitbag service refresh](kitbag_service_refresh.md)	 - Repair an installed service after an upgrade
* [kitbag service status](kitbag_service_status.md)	 - Show supervisor health and the last synchronization
* [kitbag service uninstall](kitbag_service_uninstall.md)	 - Stop background sync and remove service definitions

