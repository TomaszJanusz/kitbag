## kitbag daemon

Poll configuration, detect drift and serve the local API

### Synopsis

Run in the foreground with a per-user lock, bounded JSON logs, debounced filesystem watches and daily release metadata checks. Remote changes use the normal safe sync workflow; local edits only trigger inspection. Stop with SIGINT or SIGTERM. Restart after changing daemon settings. When its executable file is replaced, for example by a package upgrade, the daemon finishes the current operation and exits with status 75 so that its service manager starts the new version.

```
kitbag daemon [flags]
```

### Options

```
  -h, --help   help for daemon
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

