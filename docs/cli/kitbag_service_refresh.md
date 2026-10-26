## kitbag service refresh

Repair an installed service after an upgrade

### Synopsis

Compare the installed definition with this binary's generator, preserving saved installation inputs. Atomically replace stale definitions and reload/restart only as needed, including a running daemon with a different version. An uninstalled service is left alone. --dry-run reports proposed work without writing or contacting the supervisor to change it. Exit 0 on success (including no-op), 1 on failure, 2 for invalid input, 5 on unsupported systems.

```
kitbag service refresh [flags]
```

### Options

```
      --dry-run   Report changes without writing or restarting
  -h, --help      help for refresh
      --json      Print stable service_schema=1 JSON
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

