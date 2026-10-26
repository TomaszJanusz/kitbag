## kitbag service install

Enable background sync with a daemon or timer

### Synopsis

Write service definitions atomically and enable/start them. Repeating the same installation keeps a daemon only when its running version matches this binary. Older or unversioned daemons are restarted. --no-start only enables future starts; --now explicitly requests the default immediate start. Timer intervals range from 1m to 24h.

```
kitbag service install [flags]
```

### Options

```
  -h, --help                help for install
      --interval duration   Timer cadence (1m to 24h) (default 15m0s)
      --mode string         Service mode: daemon or timer (default "daemon")
      --no-start            Enable the service without starting it now
      --now                 Start immediately (the default)
```

### Options inherited from parent commands

```
      --config string        Machine configuration file (default: <config>/config.toml)
      --git-backend string   Git backend: auto, system or embedded
      --host string          Host override (before KITBAG_HOST and config host)
      --quiet                Suppress successful sync change output
      --verbose              Show backend selection and working copy diagnostics
```

### SEE ALSO

* [kitbag service](kitbag_service.md)	 - Manage background synchronization for this user

