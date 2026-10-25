## kitbag config

Inspect and change machine settings

### Synopsis

Manage config.toml with validated atomic writes. Precedence: flag > KITBAG_* environment > config.toml > default. Use edit for comment-bearing files and paired identity changes.

### Options

```
  -h, --help   help for config
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
* [kitbag config edit](kitbag_config_edit.md)	 - Edit a private draft with $EDITOR and validate before saving
* [kitbag config explain](kitbag_config_explain.md)	 - Describe a setting, default, effective value and source
* [kitbag config get](kitbag_config_get.md)	 - Print an effective value
* [kitbag config list](kitbag_config_list.md)	 - List effective values and their sources
* [kitbag config set](kitbag_config_set.md)	 - Validate and persist a setting
* [kitbag config unset](kitbag_config_unset.md)	 - Remove a stored optional setting

